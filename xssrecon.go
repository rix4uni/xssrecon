package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rix4uni/xssrecon/banner"
	"github.com/spf13/pflag"
)

const (
	payloadPrefix = "rix4uni"
	payloadSuffix = "'\"<>"
)

var outputMutex sync.Mutex
var chromedpSemaphore chan struct{}

// Thread-safe print functions
func safePrintf(format string, a ...interface{}) {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	fmt.Printf(format, a...)
}

func safePrintln(a ...interface{}) {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	fmt.Println(a...)
}

func fetch(url string, userAgent string, timeout int) (string, error) {
	// Skip TLS certificate verification
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(timeout) * time.Second,
	}

	// Create HTTP request with custom User-Agent
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bodyBytes), nil
}

func fetchDOM(url string, userAgent string, timeout int) (string, error) {
	// Acquire semaphore for concurrency control
	if chromedpSemaphore != nil {
		chromedpSemaphore <- struct{}{}
		defer func() { <-chromedpSemaphore }()
	}

	// Create this browser's profile directory ourselves (instead of letting
	// chromedp do it) so we control when it is deleted. Chrome's own temp
	// files (TMPDIR) also go inside it, so one delete removes everything.
	profileDir, err := os.MkdirTemp("", "xssrecon-chrome-")
	if err != nil {
		return "", err
	}
	if err := os.Mkdir(filepath.Join(profileDir, "tmp"), 0o700); err != nil {
		os.RemoveAll(profileDir)
		return "", err
	}
	sess := &chromeSession{dir: profileDir, tmpDir: filepath.Join(profileDir, "tmp")}
	trackSession(sess)
	// Deferred first, so it runs last: after the browser is fully closed,
	// kill any leftover Chrome processes and delete the profile.
	defer closeSession(sess)

	// Safety net in case Chrome hangs while starting or closing. The page
	// timeout itself is applied to the tab below, so a slow page closes the
	// browser gracefully instead of killing it mid-write.
	watchdogCtx, watchdogCancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second+30*time.Second)
	defer watchdogCancel()

	// Create chromedp context with options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent(userAgent),
		chromedp.UserDataDir(profileDir),
		chromedp.Env("TMPDIR="+sess.tmpDir),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(watchdogCtx, opts...)
	defer allocCancel()

	// Create chrome context
	ctx, ctxCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, v ...interface{}) {
		// Suppress chromedp logs
	}))
	defer ctxCancel()
	// Runs before ctxCancel: ask Chrome to close itself cleanly (up to 5s),
	// then kill whatever is left of its process tree.
	defer shutdownBrowser(ctx, sess)

	// Start the browser
	if err := chromedp.Do(ctx); err != nil {
		return "", err
	}
	if c := chromedp.FromContext(ctx); c != nil && c.Browser != nil {
		sess.setProcess(c.Browser.Process())
	}

	tabCtx, tabCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer tabCancel()

	// Navigate to URL and wait for JavaScript to render
	err = chromedp.Do(tabCtx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(chromedp.CSS("body")),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return "", err
	}

	// Get rendered DOM
	return chromedp.Run(tabCtx, chromedp.OuterHTML(chromedp.CSS("html")))
}

// chromeSession tracks one launched Chrome browser and the profile
// directory it uses, so the profile can be deleted once the browser
// (and every child process it spawned) has exited.
type chromeSession struct {
	dir    string
	tmpDir string // TMPDIR given to Chrome; inherited by all its processes

	mu   sync.Mutex
	proc *os.Process
}

func (s *chromeSession) setProcess(p *os.Process) {
	s.mu.Lock()
	s.proc = p
	s.mu.Unlock()
}

// killTree force-kills the browser and all of its child processes
// (renderer, GPU, network, zygote), so nothing keeps writing into the
// profile directory after we delete it.
func (s *chromeSession) killTree() {
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	if p != nil {
		_ = p.Kill()
	}
	killProcessesWithEnv("TMPDIR=" + s.tmpDir)
}

// killProcessesWithEnv kills every process whose environment contains the
// exact entry marker. Each browser gets a unique TMPDIR and every Chrome
// child process inherits it, so this finds that browser's leftover
// processes even after they were orphaned. Only Linux has /proc/<pid>/environ;
// on Windows and macOS Chrome's children exit together with the browser.
func killProcessesWithEnv(marker string) {
	if runtime.GOOS != "linux" {
		return
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	self := os.Getpid()
	want := []byte(marker)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(env, []byte{0}) {
			if bytes.Equal(kv, want) {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
				break
			}
		}
	}
}

var (
	sessionsMu sync.Mutex
	sessions   = map[*chromeSession]struct{}{}
)

func trackSession(s *chromeSession) {
	sessionsMu.Lock()
	sessions[s] = struct{}{}
	sessionsMu.Unlock()
}

func untrackSession(s *chromeSession) {
	sessionsMu.Lock()
	delete(sessions, s)
	sessionsMu.Unlock()
}

// removeProfileDir deletes the profile directory, retrying for a few
// seconds in case a dying process is still releasing or writing files.
func removeProfileDir(dir string) error {
	var err error
	for i := 0; i < 10; i++ {
		err = os.RemoveAll(dir)
		if err == nil {
			if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
				return nil
			}
			err = fmt.Errorf("directory was recreated while deleting")
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

// closeSession runs after the browser has been closed: it kills any
// leftover Chrome processes and deletes the profile directory. If the
// delete fails, the session stays tracked so cleanupAllSessions retries it
// before the program exits.
func closeSession(s *chromeSession) {
	s.killTree()
	if err := removeProfileDir(s.dir); err != nil {
		outputMutex.Lock()
		fmt.Fprintf(os.Stderr, "warning: could not delete Chrome profile %s: %v\n", s.dir, err)
		outputMutex.Unlock()
		return
	}
	untrackSession(s)
}

// cleanupAllSessions kills every browser still tracked and deletes its
// profile. Used at normal exit and when the tool is interrupted.
func cleanupAllSessions() {
	sessionsMu.Lock()
	list := make([]*chromeSession, 0, len(sessions))
	for s := range sessions {
		list = append(list, s)
	}
	sessionsMu.Unlock()

	for _, s := range list {
		s.killTree()
		if removeProfileDir(s.dir) == nil {
			untrackSession(s)
		}
	}
}

// shutdownBrowser closes Chrome gracefully, giving it up to 5 seconds,
// then force-kills any Chrome processes still alive.
func shutdownBrowser(ctx context.Context, sess *chromeSession) {
	done := make(chan struct{})
	go func() {
		chromedp.Cancel(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	sess.killTree()
}

var reParamValue = regexp.MustCompile(`=[^&\s]*`)

func replaceParamValues(u string) (string, string, []string) {
	count := 0
	var payloads []string

	displayURL := reParamValue.ReplaceAllStringFunc(u, func(match string) string {
		count++
		p := fmt.Sprintf("%s%d%s", payloadPrefix, count, payloadSuffix)
		payloads = append(payloads, p)
		return "=" + p
	})

	count = 0
	requestURL := reParamValue.ReplaceAllStringFunc(u, func(match string) string {
		count++
		p := fmt.Sprintf("%s%d%s", payloadPrefix, count, payloadSuffix)
		return "=" + url.QueryEscape(p)
	})

	return displayURL, requestURL, payloads
}

func isReflected(content string, payloads []string) bool {
	for _, p := range payloads {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

func processURL(inputURL string, userAgent string, timeout int, noColor bool, verbose bool, chromedpTimeout int, noChromeDP bool, vuln bool) {
	// Build the display URL and standard URL-encoded request URL
	displayURL, requestURL, injectedPayloads := replaceParamValues(inputURL)

	// Make a single HTTP request with standard browser-encoded payload
	body, fetchErr := fetch(requestURL, userAgent, timeout)
	reflected := false

	if fetchErr != nil {
		if verbose {
			safePrintf("Error fetching URL: %v\n", fetchErr)
		}
	} else {
		reflected = isReflected(body, injectedPayloads)
	}

	// If not found in HTML, check DOM as fallback
	if !reflected && !noChromeDP && fetchErr == nil {
		if verbose {
			if noColor {
				safePrintln("Not found in HTML, checking DOM...")
			} else {
				safePrintln("\033[36mNot found in HTML, checking DOM...\033[0m")
			}
		}
		domBody, domErr := fetchDOM(requestURL, userAgent, chromedpTimeout)
		if domErr != nil {
			if verbose {
				safePrintf("Error fetching DOM: %v\n", domErr)
			}
		} else {
			reflected = isReflected(domBody, injectedPayloads)
		}
	}

	if reflected {
		if noColor {
			safePrintf("[REFLECTED] %s\n", displayURL)
		} else {
			safePrintf("[\033[91mREFLECTED\033[0m] %s\n", displayURL)
		}
	} else if !vuln {
		if noColor {
			safePrintf("[NOT REFLECTED] %s\n", displayURL)
		} else {
			safePrintf("[\033[92mNOT REFLECTED\033[0m] %s\n", displayURL)
		}
	}
}

func main() {
	userAgent := pflag.StringP("user-agent", "H", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36", "Custom User-Agent header for HTTP requests.")
	timeout := pflag.IntP("timeout", "t", 15, "Timeout for HTTP requests in seconds.")
	concurrent := pflag.IntP("concurrent", "c", 50, "Number of concurrent workers for processing URLs (default: 50).")
	noColor := pflag.Bool("no-color", false, "Do not use colored output.")
	silent := pflag.Bool("silent", false, "silent mode.")
	version := pflag.Bool("version", false, "Print the version of the tool and exit.")
	verbose := pflag.Bool("verbose", false, "Enable verbose output for debugging purposes.")
	chromedpConcurrent := pflag.Int("chromedp-concurrent", 5, "Number of concurrent ChromeDP browser instances")
	chromedpTimeout := pflag.Int("chromedp-timeout", 30, "ChromeDP page rendering timeout in seconds")
	noChromeDP := pflag.Bool("no-chromedp", false, "Disable ChromeDP fallback")
	vuln := pflag.Bool("vuln", false, "Show only REFLECTED url")
	pflag.Parse()

	if *version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	if !*silent {
		banner.PrintBanner()
	}

	// Read all URLs from stdin
	var urls []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		url := strings.TrimSpace(scanner.Text())
		if url != "" {
			urls = append(urls, url)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		return
	}

	if len(urls) == 0 {
		return
	}

	// Initialize ChromeDP semaphore for concurrency control
	numChromeDPWorkers := *chromedpConcurrent
	if numChromeDPWorkers < 1 {
		numChromeDPWorkers = 1
	}
	chromedpSemaphore = make(chan struct{}, numChromeDPWorkers)

	// On Ctrl+C / kill, close all open browsers and delete their profiles
	// before exiting.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cleanupAllSessions()
		os.Exit(130)
	}()

	// Create worker pool
	numWorkers := *concurrent
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(urls) {
		numWorkers = len(urls)
	}

	jobs := make(chan string, len(urls))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range jobs {
				processURL(url, *userAgent, *timeout, *noColor, *verbose, *chromedpTimeout, *noChromeDP, *vuln)
			}
		}()
	}

	// Send jobs to workers
	for _, url := range urls {
		jobs <- url
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	// Retry deleting any profile that could not be removed earlier
	cleanupAllSessions()
}

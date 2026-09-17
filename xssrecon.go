package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rix4uni/xssrecon/banner"
	"github.com/spf13/pflag"
)

const (
	payloadPrefix = "rix4uni"
	payloadSuffix = "'\">"
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

	// Create context with timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer timeoutCancel()

	// Create chromedp context with options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent(userAgent),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
	defer allocCancel()

	// Create chrome context
	ctx, ctxCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, v ...interface{}) {
		// Suppress chromedp logs
	}))
	defer ctxCancel()

	var htmlContent string

	// Navigate to URL and get rendered DOM
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second), // Wait for JavaScript to render
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err != nil {
		return "", err
	}

	return htmlContent, nil
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
}

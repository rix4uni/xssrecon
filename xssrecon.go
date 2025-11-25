package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rix4uni/xssrecon/banner"
	"github.com/spf13/pflag"
)

var specialChars = []string{`'`, `"`, `<`, `>`, `(`, `)`, "`", `{`, `}`, `/`, `\`, `;`}

var outputMutex sync.Mutex

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

type JSONOutput struct {
	Processing string         `json:"processing"`
	BaseURL    string         `json:"baseurl"`
	Reflected  bool           `json:"reflected"`
	Allowed    []string       `json:"allowed"`
	Blocked    []string       `json:"blocked"`
	Converted  []string       `json:"converted"`
	Count      map[string]int `json:"count"`
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

func runPvreplace(inputURL, payload string) (string, error) {
	cmd := exec.Command("pvreplace", "-silent", "-payload", payload, "-fuzzing-mode", "single")
	cmd.Stdin = strings.NewReader(inputURL)

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out.String()), nil
}

var conversions = map[string]string{
	"'": "&#039;",
	`"`: "&quot;",
	"<": "&lt;",
	">": "&gt;",
}

func parseSpecialChars(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	result := []string{}

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func processURL(inputURL string, userAgent string, timeout int, noColor bool, verbose bool, skipSpecialChar bool, jsonOutput bool, customSpecialChars []string) {
	if !jsonOutput {
		if noColor {
			safePrintf("\nPROCESSING: %s\n", inputURL)
		} else {
			safePrintf("\n\033[96mPROCESSING: %s\033[0m\n", inputURL)
		}
	}

	baseURLsRaw, err := runPvreplace(inputURL, "rix4uni")
	if err != nil {
		safePrintf("Error running pvreplace: %v\n", err)
		return
	}

	baseURLs := strings.Split(baseURLsRaw, "\n")

	for _, baseURL := range baseURLs {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			continue
		}

		var output JSONOutput
		output.Processing = inputURL
		output.BaseURL = baseURL

		if !jsonOutput {
			if noColor {
				safePrintf("BASEURL: %s\n", baseURL)
			} else {
				safePrintf("\033[94mBASEURL: %s\033[0m\n", baseURL)
			}
		}

		body, err := fetch(baseURL, userAgent, timeout)
		if err != nil {
			if verbose {
				safePrintf("Error fetching base URL: %v\n", err)
			}
			continue
		}

		reflected := strings.Contains(body, "rix4uni")
		var domBody string
		baseFoundInHTML := reflected
		baseFoundInDOM := false

		// If not found in HTML, check DOM as fallback
		if !reflected {
			if verbose && !jsonOutput {
				if noColor {
					safePrintln("Not found in HTML, checking DOM...")
				} else {
					safePrintln("\033[36mNot found in HTML, checking DOM...\033[0m")
				}
			}
			domBody, err = fetchDOM(baseURL, userAgent, timeout)
			if err != nil {
				if verbose {
					safePrintf("Error fetching DOM: %v\n", err)
				}
			} else {
				reflected = strings.Contains(domBody, "rix4uni")
				baseFoundInDOM = reflected
			}
		}

		if reflected {
			output.Reflected = true
			if !jsonOutput {
				if noColor {
					safePrintln("REFLECTED: YES")
				} else {
					safePrintln("\033[92mREFLECTED: YES\033[0m")
				}
			}

			// If skipSpecialChar is set, skip the rest
			if skipSpecialChar {
				if jsonOutput {
					output.Allowed = []string{}
					output.Blocked = []string{}
					output.Converted = []string{}
					output.Count = map[string]int{"allowed": 0, "blocked": 0, "converted": 0}
					jsonBytes, _ := json.MarshalIndent(output, "", "  ")
					safePrintln(string(jsonBytes))
				}
				continue
			}

			// Otherwise, continue with special char checks
			allowed := []string{}
			blocked := []string{}
			converted := []string{}

			for _, char := range customSpecialChars {
				testURLRaw, err := runPvreplace(baseURL, "rix4uni"+char)
				if err != nil {
					safePrintf("Error running pvreplace for char '%s': %v\n", char, err)
					continue
				}

				testURLs := strings.Split(testURLRaw, "\n")
				for _, testURL := range testURLs {
					testURL = strings.TrimSpace(testURL)
					if testURL == "" {
						continue
					}

					if verbose && !jsonOutput {
						if noColor {
							safePrintf("CHECKING: %s\n", testURL)
						} else {
							safePrintf("\033[95mCHECKING: %s\033[0m\n", testURL)
						}
					}

					var testBody string
					var testDomBody string
					foundInHTML := false
					foundInDOM := false

					// If base URL was only found in DOM, skip HTML check and go straight to DOM
					if baseFoundInDOM && !baseFoundInHTML {
						testDomBody, err = fetchDOM(testURL, userAgent, timeout)
						if err != nil {
							if verbose {
								safePrintf("Error fetching DOM for test URL: %v\n", err)
							}
							continue
						} else {
							foundInDOM = strings.Contains(testDomBody, "rix4uni"+char)
						}
					} else {
						// Base URL was found in HTML, so check HTML first
						testBody, err = fetch(testURL, userAgent, timeout)
						if err != nil {
							if verbose {
								safePrintf("Error fetching test URL: %v\n", err)
							}
							continue
						}

						foundInHTML = strings.Contains(testBody, "rix4uni"+char)

						// If not found in HTML, check DOM as fallback
						if !foundInHTML {
							testDomBody, err = fetchDOM(testURL, userAgent, timeout)
							if err != nil {
								if verbose {
									safePrintf("Error fetching DOM for test URL: %v\n", err)
								}
							} else {
								foundInDOM = strings.Contains(testDomBody, "rix4uni"+char)
							}
						}
					}

					if foundInHTML || foundInDOM {
						allowed = append(allowed, char) // ALLOWED
					} else if conv, exists := conversions[char]; exists {
						convertedInHTML := false
						convertedInDOM := false

						// If base URL was only found in DOM, skip HTML check for converted entities
						if baseFoundInDOM && !baseFoundInHTML {
							// We already have testDomBody from the character check above
							if testDomBody != "" {
								convertedInDOM = strings.Contains(testDomBody, "rix4uni"+conv)
							}
						} else {
							// Base URL was found in HTML, so check HTML first
							convertedInHTML = strings.Contains(testBody, "rix4uni"+conv)

							// If not found in HTML, check DOM for converted entities
							if !convertedInHTML && testDomBody != "" {
								convertedInDOM = strings.Contains(testDomBody, "rix4uni"+conv)
							} else if !convertedInHTML {
								// Fetch DOM if we haven't already
								testDomBody, err = fetchDOM(testURL, userAgent, timeout)
								if err == nil {
									convertedInDOM = strings.Contains(testDomBody, "rix4uni"+conv)
								}
							}
						}

						if convertedInHTML || convertedInDOM {
							converted = append(converted, fmt.Sprintf("%s ➔ %s", char, conv)) // CONVERTED
						} else {
							blocked = append(blocked, char) // BLOCKED
						}
					} else {
						blocked = append(blocked, char) // BLOCKED
					}

					break // Test first generated URL only
				}
			}

			if jsonOutput {
				output.Allowed = allowed
				output.Blocked = blocked
				output.Converted = converted
				output.Count = map[string]int{
					"allowed":   len(allowed),
					"blocked":   len(blocked),
					"converted": len(converted),
				}
				jsonBytes, _ := json.MarshalIndent(output, "", "  ")
				safePrintln(string(jsonBytes))
			} else {
				if noColor {
					safePrintf("ALLOWED: %v\n", allowed)
					safePrintf("BLOCKED: %v\n", blocked)
					safePrintf("CONVERTED: %v\n", converted)
				} else {
					safePrintf("\033[32mALLOWED: %v\033[0m\n", allowed)
					safePrintf("\033[31mBLOCKED: %v\033[0m\n", blocked)
					safePrintf("\033[33mCONVERTED: %v\033[0m\n", converted)
				}
			}

		} else {
			output.Reflected = false
			if jsonOutput {
				output.Allowed = []string{}
				output.Blocked = []string{}
				output.Converted = []string{}
				output.Count = map[string]int{"allowed": 0, "blocked": 0, "converted": 0}
				jsonBytes, _ := json.MarshalIndent(output, "", "  ")
				safePrintln(string(jsonBytes))
			} else {
				if noColor {
					safePrintln("REFLECTED: NO")
				} else {
					safePrintln("\033[91mREFLECTED: NO\033[0m")
				}
			}
		}
	}
}

func main() {
	userAgent := pflag.StringP("user-agent", "H", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36", "Custom User-Agent header for HTTP requests.")
	timeout := pflag.IntP("timeout", "t", 30, "Timeout for HTTP requests in seconds.")
	skipSpecialChar := pflag.BoolP("skipspecialchar", "s", false, "Only check rix4uni in reponse and move to next url, skip checking special characters.")
	specialChar := pflag.String("specialchar", "", "Custom special characters to test (single char or comma-separated, e.g., '<' or '<, >'). Cannot be used with --skipspecialchar.")
	concurrent := pflag.IntP("concurrent", "c", 10, "Number of concurrent workers for processing URLs (default: 10).")
	noColor := pflag.Bool("no-color", false, "Do not use colored output.")
	silent := pflag.Bool("silent", false, "silent mode.")
	version := pflag.Bool("version", false, "Print the version of the tool and exit.")
	verbose := pflag.Bool("verbose", false, "Enable verbose output for debugging purposes.")
	jsonOutput := pflag.Bool("json", false, "Output results in JSON format.")
	pflag.Parse()

	if *version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	// Validate that --specialchar and --skipspecialchar are not used together
	if *specialChar != "" && *skipSpecialChar {
		fmt.Fprintf(os.Stderr, "Error: --specialchar and --skipspecialchar cannot be used together\n")
		os.Exit(1)
	}

	// Parse custom special characters if provided
	var charsToUse []string
	if *specialChar != "" {
		charsToUse = parseSpecialChars(*specialChar)
		if len(charsToUse) == 0 {
			fmt.Fprintf(os.Stderr, "Error: --specialchar provided but no valid characters found\n")
			os.Exit(1)
		}
	} else {
		charsToUse = specialChars
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
				processURL(url, *userAgent, *timeout, *noColor, *verbose, *skipSpecialChar, *jsonOutput, charsToUse)
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

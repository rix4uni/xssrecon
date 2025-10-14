package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/rix4uni/xssrecon/banner"
)

var specialChars = []string{`'`, `"`, `<`, `>`, `(`, `)`, "`", `{`, `}`, `/`, `\`, `;`}

type JSONOutput struct {
	Processing string            `json:"processing"`
	BaseURL    string            `json:"baseurl"`
	Reflected  bool              `json:"reflected"`
	Allowed    []string          `json:"allowed"`
	Blocked    []string          `json:"blocked"`
	Converted  []string          `json:"converted"`
	Count      map[string]int    `json:"count"`
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
	"'":  "&#039;",
	`"`:  "&quot;",
	"<":  "&lt;",
	">":  "&gt;",
}

func processURL(inputURL string, userAgent string, timeout int, noColor bool, verbose bool, skipSpecialChar bool, jsonOutput bool) {
	if !jsonOutput {
		if noColor {
			fmt.Printf("\nPROCESSING: %s\n", inputURL)
		} else {
			fmt.Printf("\n\033[96mPROCESSING: %s\033[0m\n", inputURL)
		}
	}

	baseURLsRaw, err := runPvreplace(inputURL, "rix4uni")
	if err != nil {
		fmt.Printf("Error running pvreplace: %v\n", err)
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
				fmt.Printf("BASEURL: %s\n", baseURL)
			} else {
				fmt.Printf("\033[94mBASEURL: %s\033[0m\n", baseURL)
			}
		}

		body, err := fetch(baseURL, userAgent, timeout)
		if err != nil {
			if verbose {
				fmt.Printf("Error fetching base URL: %v\n", err)
			}
			continue
		}

		if strings.Contains(body, "rix4uni") {
			output.Reflected = true
			if !jsonOutput {
				if noColor {
					fmt.Println("REFLECTED: YES")
				} else {
					fmt.Println("\033[92mREFLECTED: YES\033[0m")
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
					fmt.Println(string(jsonBytes))
				}
				continue
			}

			// Otherwise, continue with special char checks
			allowed := []string{}
			blocked := []string{}
			converted := []string{}

			for _, char := range specialChars {
				testURLRaw, err := runPvreplace(baseURL, "rix4uni"+char)
				if err != nil {
					fmt.Printf("Error running pvreplace for char '%s': %v\n", char, err)
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
							fmt.Printf("CHECKING: %s\n", testURL)
						} else {
							fmt.Printf("\033[95mCHECKING: %s\033[0m\n", testURL)
						}
					}

					testBody, err := fetch(testURL, userAgent, timeout)
					if err != nil {
						if verbose {
							fmt.Printf("Error fetching test URL: %v\n", err)
						}
						continue
					}

					if strings.Contains(testBody, "rix4uni"+char) {
						allowed = append(allowed, char) // ALLOWED
					} else if conv, exists := conversions[char]; exists && strings.Contains(testBody, "rix4uni"+conv) {
						converted = append(converted, fmt.Sprintf("%s ➔ %s", char, conv)) // CONVERTED
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
				fmt.Println(string(jsonBytes))
			} else {
				if noColor {
					fmt.Printf("ALLOWED: %v\n", allowed)
					fmt.Printf("BLOCKED: %v\n", blocked)
					fmt.Printf("CONVERTED: %v\n", converted)
				} else {
					fmt.Printf("\033[32mALLOWED: %v\033[0m\n", allowed)
					fmt.Printf("\033[31mBLOCKED: %v\033[0m\n", blocked)
					fmt.Printf("\033[33mCONVERTED: %v\033[0m\n", converted)
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
				fmt.Println(string(jsonBytes))
			} else {
				if noColor {
					fmt.Println("REFLECTED: NO")
				} else {
					fmt.Println("\033[91mREFLECTED: NO\033[0m")
				}
			}
		}
	}
}

func main() {
	userAgent := pflag.StringP("user-agent", "H", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36", "Custom User-Agent header for HTTP requests.")
	timeout := pflag.IntP("timeout", "t", 15, "Timeout for HTTP requests in seconds.")
	skipSpecialChar := pflag.BoolP("skipspecialchar", "s", false, "Only check rix4uni in reponse and move to next url, skip checking special characters.")
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

	if !*silent {
		banner.PrintBanner()
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputURL := scanner.Text()
		processURL(inputURL, *userAgent, *timeout, *noColor, *verbose, *skipSpecialChar, *jsonOutput)
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading input: %v\n", err)
	}
}

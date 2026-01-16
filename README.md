## XSSRecon - Automated Reflected XSS Parameter Discovery Tool

**XSSRecon** is a powerful tool designed to help security researchers and penetration testers identify reflected XSS vulnerabilities in web applications.
It automates the process of testing URL parameters for reflection of a test payload (`rix4uni`), and further checks how special characters are handled (allowed, blocked, or converted).

### 🚀 Features:

* **Dual Detection Method**: Detects if input is reflected in both HTTP response body and rendered DOM
* **DOM Checking**: Automatically checks JavaScript-rendered content using headless Chrome (chromedp) with configurable concurrency and timeout
* **Smart Optimization**: Skips HTML checks for special characters when base URL is only found in DOM
* **Custom Special Characters**: Test custom special characters with `--specialchar` flag
* **Concurrent Processing**: Process multiple URLs in parallel with configurable worker pool (default: 50)
* **ChromeDP Concurrency Control**: Limit concurrent ChromeDP browser instances to manage resource usage (default: 5)
* **Special Character Testing**: Tests special characters for allowed, blocked, or converted behavior
* **Flexible Output**: Supports JSON output, colorized or plain text, silent and verbose modes
* **Parameter Injection**: Uses external `pvreplace` tool for precise parameter injection
* **ChromeDP Control**: Disable DOM checking entirely with `--no-chromedp` flag for faster execution when only HTML checking is needed

## Prerequisites

* **pvreplace**: Required for parameter injection
  ```
  go install github.com/rix4uni/pvreplace@latest
  ```

* **Chrome/Chromium**: Required for DOM checking (automatically used when HTML check fails)
  - Chrome or Chromium must be installed on your system
  - The tool will automatically use it for DOM-based reflection detection

## Installation

```
go install github.com/rix4uni/xssrecon@latest
```

## Download prebuilt binaries

```
wget https://github.com/rix4uni/xssrecon/releases/download/v0.0.4/xssrecon-linux-amd64-0.0.4.tgz
tar -xvzf xssrecon-linux-amd64-0.0.4.tgz
rm -rf xssrecon-linux-amd64-0.0.4.tgz
mv xssrecon ~/go/bin/xssrecon
```

Or download [binary release](https://github.com/rix4uni/xssrecon/releases) for your platform.

## Compile from source

```
git clone --depth 1 https://github.com/rix4uni/xssrecon.git
cd xssrecon; go install
```

## Usage

```yaml
Usage of xssrecon:
  -c, --concurrent int           Number of concurrent workers for processing URLs (default: 50)
  -H, --user-agent string        Custom User-Agent header for HTTP requests. (default "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")
  -s, --skipspecialchar          Only check rix4uni in response and move to next url, skip checking special characters.
  -t, --timeout int              Timeout for HTTP requests in seconds. (default 15)
      --chromedp-concurrent int  Number of concurrent ChromeDP browser instances (default 5)
      --chromedp-timeout int     ChromeDP page rendering timeout in seconds (default 30)
      --no-chromedp              Disable ChromeDP fallback
      --json                     Output results in JSON format.
      --no-color                 Do not use colored output.
      --silent                   silent mode.
      --specialchar string       Custom special characters to test (single char or comma-separated, e.g., '<' or '<, >'). Cannot be used with --skipspecialchar.
      --verbose                  Enable verbose output for debugging purposes.
      --version                  Print the version of the tool and exit.
```

## Usage Examples

### Single URL:
```yaml
echo "https://labs.hackxpert.com/RXSS/GET/01.php?fname=rat" | xssrecon
```

### Multiple URLs with concurrent processing:
```yaml
cat urls.txt | xssrecon --concurrent 20
```

### Custom special characters:
```yaml
cat urls.txt | xssrecon --specialchar "<, >"
```

### Verbose mode (shows DOM checking):
```yaml
echo "https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni" | xssrecon --verbose
```

### Silent mode with JSON output:
```yaml
cat urls.txt | xssrecon --silent --json
```

### Skip special character checks (faster):
```yaml
cat urls.txt | xssrecon --silent --skipspecialchar --json
```

### Disable ChromeDP DOM checking (faster, HTML only):
```yaml
cat urls.txt | xssrecon --no-chromedp
```

### Configure ChromeDP concurrency and timeout:
```yaml
cat urls.txt | xssrecon --chromedp-concurrent 10 --chromedp-timeout 60
```

## Output Examples

### Standard Output:
```yaml
PROCESSING: https://labs.hackxpert.com/RXSS/GET/01.php?fname=rat
BASEURL: https://labs.hackxpert.com/RXSS/GET/01.php?fname=rix4uni
REFLECTED: YES
ALLOWED: [' " < > ( ) ` { } / \ ;]
BLOCKED: []
CONVERTED: []
```

### Verbose Output (DOM detection):
```yaml
PROCESSING: https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni
BASEURL: https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni
Not found in HTML, checking DOM...
REFLECTED: YES
CHECKING: https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni'
CHECKING: https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni"
...
ALLOWED: [' " ( ) ` { } / \ ;]
BLOCKED: []
CONVERTED: [< ➔ &lt; > ➔ &gt;]
```

### JSON Output:
```json
{
  "processing": "https://labs.hackxpert.com/RXSS/GET/01.php?fname=rat",
  "baseurl": "https://labs.hackxpert.com/RXSS/GET/01.php?fname=rix4uni",
  "reflected": true,
  "allowed": ["'", "\"", "<", ">", "(", ")", "`", "{", "}", "/", "\\", ";"],
  "blocked": [],
  "converted": [],
  "count": {
    "allowed": 12,
    "blocked": 0,
    "converted": 0
  }
}
```

### Filtering with jq:
```yaml
# Get only reflected URLs
cat urls.txt | xssrecon --silent --skipspecialchar --json | jq -r 'select(.reflected==true) | .baseurl'

# Get URLs with all special characters allowed
cat urls.txt | xssrecon --silent --json | jq -r 'select(.reflected==true) | select(.count.allowed==12) | .baseurl'
```

## How It Works

1. **Parameter Injection**: Uses `pvreplace` to inject the test payload `rix4uni` into URL parameters
2. **HTML Check**: First checks if the payload is reflected in the raw HTTP response body
3. **DOM Check**: If not found in HTML (and `--no-chromedp` is not set), automatically checks the rendered DOM using headless Chrome with configurable concurrency and timeout
4. **Special Character Testing**: Tests each special character to determine if it's allowed, blocked, or converted
5. **Optimization**: If base URL is only found in DOM, skips HTML checks for special characters (faster execution)
6. **Concurrent Processing**: Processes multiple URLs in parallel using a worker pool (configurable with `--concurrent`)
7. **ChromeDP Concurrency Control**: ChromeDP operations are limited by `--chromedp-concurrent` to prevent resource exhaustion

## Notes

* DOM checking requires Chrome/Chromium to be installed on your system
* If Chrome is not available, DOM checking will fail gracefully and only HTML checking will be performed
* The `--concurrent` flag controls how many URLs are processed simultaneously (default: 50)
* The `--chromedp-concurrent` flag controls how many ChromeDP browser instances run concurrently (default: 5)
* Use `--no-chromedp` to disable DOM checking entirely for faster execution when only HTML checking is needed
* Use `--chromedp-timeout` to adjust the timeout for ChromeDP page rendering (default: 30 seconds)
* Use `--specialchar` to test specific characters instead of the default set
* `--specialchar` and `--skipspecialchar` cannot be used together
* The default HTTP request timeout is 15 seconds (configurable with `-t` or `--timeout`)

<img width="792" height="820" alt="image" src="https://github.com/user-attachments/assets/3209c95f-cb7f-4f15-b85e-dd25c4b490a2" />

## XSSRecon

XSSRecon is a fast reflected-parameter discovery tool that uses unique numbered payloads to detect input reflection in HTTP responses and JavaScript-rendered DOMs.

## Prerequisites
* **Chrome/Chromium**: Required for DOM checking (automatically used when HTML check fails)
  - Chrome or Chromium must be installed on your system
  - The tool will automatically use it for DOM-based reflection detection

## Installation

```
go install github.com/rix4uni/xssrecon@latest
```

## Download prebuilt binaries

```
wget https://github.com/rix4uni/xssrecon/releases/download/v0.0.6/xssrecon-linux-amd64-0.0.6.tgz
tar -xvzf xssrecon-linux-amd64-0.0.6.tgz
rm -rf xssrecon-linux-amd64-0.0.6.tgz
mv xssrecon ~/go/bin/xssrecon
```

Or download [binary release](https://github.com/rix4uni/xssrecon/releases) for your platform.

## Compile from source

```
git clone --depth 1 https://github.com/rix4uni/xssrecon.git
cd xssrecon; go install
```

## Usage

```console
Usage of xssrecon:
      --chromedp-concurrent int   Number of concurrent ChromeDP browser instances (default 5)
      --chromedp-timeout int      ChromeDP page rendering timeout in seconds (default 30)
  -c, --concurrent int            Number of concurrent workers for processing URLs (default: 50). (default 50)
      --no-chromedp               Disable ChromeDP fallback
      --no-color                  Do not use colored output.
      --silent                    silent mode.
  -t, --timeout int               Timeout for HTTP requests in seconds. (default 15)
  -H, --user-agent string         Custom User-Agent header for HTTP requests. (default "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")
      --verbose                   Enable verbose output for debugging purposes.
      --version                   Print the version of the tool and exit.
      --vuln                      Show only REFLECTED url
```

## Usage Examples

### Single URL
```console
echo "https://kzlabs.in/subdomains/tutorialrepublic/?q=hghjhk" | xssrecon
```

### Multiple URLs
```console
cat urls.txt | xssrecon
```

## Output
```console
[REFLECTED] https://labs.hackxpert.com/RXSS/GET/01.php?fname=rix4uni1'">
[REFLECTED] https://kzlabs.in/subdomains/tutorialrepublic/?q=rix4uni1'">
[NOT REFLECTED] http://testphp.vulnweb.com/artists.php?artist=rix4uni1'">&id=rix4uni2'">
[NOT REFLECTED] http://testphp.vulnweb.com/artists.php?file=rix4uni1'">
[NOT REFLECTED] https://labs.hackxpert.com/RXSS/GET/00.php?fname=rix4uni1'">
[NOT REFLECTED] https://summer.harvard.edu/search/?live_global%5Bquery%5D=rix4uni1'">
```
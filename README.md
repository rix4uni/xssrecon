## XSSRecon - Automated Reflected XSS Parameter Discovery Tool

**XSSRecon** is a simple and effective tool designed to help security researchers and penetration testers identify reflected XSS vulnerabilities in web applications.
It automates the process of testing URL parameters for reflection of a test payload (`rix4uni`), and further checks how special characters are handled (allowed, blocked, or converted).

### 🚀 Features:

* Detects if input is reflected in HTTP response
* Tests special characters for allowed, blocked, or converted behavior
* Supports custom User-Agent and timeout settings
* Provides colorized or plain output
* Silent and verbose modes for flexible use
* Uses external `pvreplace` tool for precise parameter injection

## Prerequisites
```
go install github.com/rix4uni/pvreplace@latest
```

## Installation
```
go install github.com/rix4uni/xssrecon@latest
```

## Download prebuilt binaries
```
wget https://github.com/rix4uni/xssrecon/releases/download/v0.0.2/xssrecon-linux-amd64-0.0.2.tgz
tar -xvzf xssrecon-linux-amd64-0.0.2.tgz
rm -rf xssrecon-linux-amd64-0.0.2.tgz
mv xssrecon ~/go/bin/xssrecon
```
Or download [binary release](https://github.com/rix4uni/xssrecon/releases) for your platform.

## Compile from source
```
git clone --depth 1 github.com/rix4uni/xssrecon.git
cd xssrecon; go install
```

## Usage
```yaml
Usage of xssrecon:
      --json                Output results in JSON format.
      --no-color            Do not use colored output.
      --silent              silent mode.
  -s, --skipspecialchar     Only check rix4uni in reponse and move to next url, skip checking special characters.
  -t, --timeout int         Timeout for HTTP requests in seconds. (default 15)
  -H, --user-agent string   Custom User-Agent header for HTTP requests. (default "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")
      --verbose             Enable verbose output for debugging purposes.
      --version             Print the version of the tool and exit.
```

## Usage Examples

Single URL:
```yaml
echo "https://labs.hackxpert.com/RXSS/GET/01.php?fname=rat" | xssrecon
```

Multiple URLs:
```yaml
cat urls.txt | xssrecon
```

## Output Examples
```yaml
Top XSS payloads is: 50
urls.txt: 4

Saved sending 100 requests:
cat urls.txt | xssrecon --silent --skipspecialchar --json | jq -r 'select(.reflected==true) | .baseurl'
https://labs.hackxpert.com/RXSS/GET/01.php?fname=rix4uni
https://labs.hackxpert.com/RXSS/GET/00.php?fname=rix4uni
http://testphp.vulnweb.com/artists.php?artist=rix4uni&id=2

Saved sending 200 requests:
cat urls.txt | xssrecon --silent --json | jq -r 'select(.reflected==true) | select(.count.allowed==12) | .baseurl'
https://labs.hackxpert.com/RXSS/GET/01.php?fname=rix4uni
```

<img width="792" height="820" alt="image" src="https://github.com/user-attachments/assets/3209c95f-cb7f-4f15-b85e-dd25c4b490a2" />

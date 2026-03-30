# All-in-One Tor Scraper

A comprehensive web scraper designed for archiving content from dark web libraries like Z-Library, Imperial Library, and similar resources. This tool automatically downloads virtually any file type it encounters and organizes them into categorized folders.

## Description

This scraper is built to handle the diverse content found on dark web libraries. It uses Playwright to browse .onion sites through the Tor network and captures downloadable files across **7 major categories** with support for dozens of file extensions:

| Category | Supported Extensions |
|----------|---------------------|
| **Videos** | .mp4, .webm, .mkv, .mov, .m4v, .avi, .flv, .wmv, .mpg, .mpeg, .ogv, .3gp, .3g2, .ts, .m3u8, .f4v |
| **Documents** | .pdf, .epub, .mobi, .azw, .azw3, .djvu, .djv, .txt, .rtf, .doc, .docx, .odt, .chm, .cbr, .cbz, .xls, .xlsx, .ppt, .pptx, .csv, .md, .tex |
| **Archives** | .zip, .rar, .7z, .tar, .gz, .bz2, .xz, .iso, .tgz, .tbz, .txz, .lz, .lzma |
| **Audio** | .mp3, .wav, .flac, .aac, .ogg, .m4a, .wma, .opus, .aiff, .au, .ra, .ram |
| **Images** | .jpg, .jpeg, .png, .gif, .bmp, .webp, .svg, .ico, .tiff, .tif, .raw, .cr2, .nef, .heic |
| **Code** | .html, .htm, .css, .js, .json, .xml, .yaml, .yml, .py, .go, .c, .cpp, .h, .java, .rb, .php, .sh, .bat, .ps1, .sql, .log |
| **Executables** | .exe, .msi, .dmg, .pkg, .deb, .rpm, .appimage, .bin, .run |

The scraper features intelligent file detection, automatic resume capability, progress bars, organized folder structure by source domain, and multi-port Tor support for parallel operations.

---

## Prerequisites

Before running the scraper, you need to install several dependencies and configure Tor.

### Required Software

| Software | Purpose | Download Link |
|----------|---------|---------------|
| **Go 1.21+** | Build and run the scraper | [golang.org/dl](https://golang.org/dl/) |
| **Tor Expert Bundle** | Anonymous routing through Tor network | [torproject.org](https://www.torproject.org/download/tor/) |
| **Git** | Clone dependencies (optional) | [git-scm.com](https://git-scm.com/download/win) |

### Step 1: Install Go

1. Download Go from [golang.org/dl](https://golang.org/dl/)
2. Run the installer (reboot after installation)
3. Verify installation:
   ```powershell
   go version
   # Should show: go version go1.21.x windows/amd64
   ```

### Step 2: Install Playwright Dependencies

The scraper uses Playwright for browser automation. Install the required browsers:

```powershell
# After installing Go, run:
go install github.com/playwright-community/playwright-go/cmd/playwright@latest

# Install browser binaries (Chromium is required)
playwright install chromium
```

If `playwright` command is not found, add Go bin to PATH:
```powershell
$env:Path += ";C:\Users\$env:USERNAME\go\bin"
```

### Step 3: Tor Expert Bundle Setup

1. Download **Tor Expert Bundle** for Windows from [torproject.org](https://www.torproject.org/download/tor/)
2. Extract to `C:\tor-expert-bundle-windows-x86_64-15.0.7\`
3. Configure the `torrc` file (see Tor Configuration below)

**Tor Configuration Repository**: [https://github.com/SkunkApe-jp/my-torrc-config.git](https://github.com/SkunkApe-jp/my-torrc-config.git)

> **Note**: The `torrc` file needs to be created inside `tor-expert-bundle/data/torrc-defaults` and won't be present by default. Follow the instructions in the linked repository for complete Tor Expert Bundle setup.

#### Configure Tor Service (Run in Administrator PowerShell)

First, run `tor.exe` in the Tor folder to initiate the service, then stop it before making these changes:

```powershell
# Update the service binPath to point to your torrc
sc.exe config tor binPath= "`"C:\tor-expert-bundle-windows-x86_64-15.0.7\tor\tor.exe`" --nt-service -f `"C:\tor-expert-bundle-windows-x86_64-15.0.7\Data\torrc`""

# Restart to apply
Restart-Service tor
```

#### Verify Tor is Running

After restarting the service, verify all ports are listening:

```powershell
Get-NetTCPConnection -LocalPort 9050, 9051, 9052, 9053, 9054
```

*If you see "Listen" for all five ports, you are ready to scrape!*

---

## How to Run

### Basic Usage

```bash
go run all_in_one_scraper.go
```

### With Custom Options

```bash
go run all_in_one_scraper.go -targets ../all_targets.yaml -output ../scraped_all -ports 9050,9051,9052 -workers 3
```

### Required File Structure

The scraper expects an `all_targets.yaml` file in the parent directory by default. The file should contain one .onion URL per line:

```yaml
# ../all_targets.yaml
example1.onion
example2.onion
example3.onion
```

**Default file paths** (relative to the scraper location):
- **Targets file**: `../all_targets.yaml`
- **Output directory**: `../scraped_all/`
- **Log file**: `../logs/all_in_one_scraper.log`

### Available Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-targets` | `../all_targets.yaml` | Path to targets file |
| `-output` | `../scraped_all` | Directory to save downloads |
| `-log` | `../logs/all_in_one_scraper.log` | Path to log file |
| `-ports` | `9050` | Comma-separated Tor SOCKS ports |
| `-workers` | `1` | Number of parallel workers |
| `-depth` | `1` | Scrape depth |
| `-fast` | `false` | Fast mode (reduce stealth delays) |
| `-resume-files` | `true` | Skip already downloaded files |
| `-no-js` | `true` | Disable JavaScript (default for safety/speed) |
| `-js` | `false` | Enable JavaScript for dynamic sites |
| `-inter-delay` | `0` | Inter-site delay (0=Gaussian 8-15min) |
| `-intra-delay` | `0` | Intra-page delay (0=60-120sec) |
| `-page-load-wait` | `0` | Seconds to wait after page load |

---

## Build from Source

### Option 1: Run Directly (Development)

```powershell
cd C:\scraper1\go_scripts\playwright\violent-tor-scraper
go mod init violent-tor-scraper 2>$null; go get github.com/playwright-community/playwright-go golang.org/x/net/proxy
go run all_in_one_scraper.go
```

### Option 2: Build EXE (Distribution)

Build a standalone Windows executable:

```powershell
cd C:\scraper1\go_scripts\playwright\violent-tor-scraper

# Initialize module and get dependencies
go mod init violent-tor-scraper
go get github.com/playwright-community/playwright-go golang.org/x/net/proxy

# Build optimized Windows x64 EXE
go build -ldflags="-s -w" -o all_in_one_scraper.exe all_in_one_scraper.go

# Verify the EXE was created
ls all_in_one_scraper.exe
```

**Build flags explained:**
- `-ldflags="-s -w"` - Strip debug info and symbol table (smaller EXE)
- `-o` - Output filename

#### What the EXE Contains

The built EXE includes:
- ✅ All Go code compiled to native machine code
- ✅ Scraper logic and file handlers
- ❌ **Not included**: Playwright browser binaries (must be installed separately)
- ❌ **Not included**: Tor Expert Bundle (must be installed separately)

**Users of the EXE still need to install:**
1. Playwright browsers: `playwright install chromium`
2. Tor Expert Bundle with configured `torrc`

---

## Creating GitHub Releases

To publish the EXE for others:

### Manual Release (One-time)

1. Build the EXE (see above)
2. Go to GitHub repo → **Releases** → **Create a new release**
3. Tag: `v1.0.0`
4. Title: `Release v1.0.0 - Windows x64`
5. Attach: `all_in_one_scraper.exe`
6. Add release notes

### Automated Releases (GitHub Actions)

Create `.github/workflows/release.yml`:

```yaml
name: Build and Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Build EXE
        run: |
          go mod init violent-tor-scraper
          go get github.com/playwright-community/playwright-go golang.org/x/net/proxy
          go build -ldflags="-s -w" -o all_in_one_scraper.exe all_in_one_scraper.go
      
      - name: Upload Release
        uses: softprops/action-gh-release@v1
        with:
          files: all_in_one_scraper.exe
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Then push a tag to trigger:

```powershell
git tag v1.0.0
git push origin v1.0.0
```

---

## Folder Structure

Downloads are organized as follows:

```
scraped_all/
├── videos/
│   └── [domain_folder]/
├── documents/
│   └── [domain_folder]/
├── archives/
│   └── [domain_folder]/
├── audio/
│   └── [domain_folder]/
├── images/
│   └── [domain_folder]/
├── code/
│   └── [domain_folder]/
└── executables/
    └── [domain_folder]/
```

---

## Resuming Interrupted Scrapes

The easiest way to resume is to simply **run the script again**. The scraper includes intelligent resume functionality:

### How Resume Works

- **`-resume-files` flag (default: true)**: Automatically skips files that have already been downloaded and exist in the output directory
- **Progress bars show [RESUME]**: When resuming a partial download
- **Log tracking**: Failed downloads are logged for manual review

### Common Reasons for Script Exit

The script may exit or need restarting due to:

1. **Network timeouts** - Slow Tor connections or unreachable sites
2. **Tor circuit failures** - Individual port connection issues
3. **Memory constraints** - Large file downloads or long-running sessions
4. **Rate limiting** - Some sites throttle connections
5. **Playwright crashes** - Browser context errors on problematic pages
6. **Power/interrupts** - System shutdowns or Ctrl+C

**Simply run the command again** - the scraper will pick up where it left off, skipping completed files and resuming partial downloads when servers support HTTP Range requests.

---

## Test Run Output Example

When you run the scraper, you'll see output like this:

```
[CHECK] Verifying Tor connection...
[CHECK] Starting All-in-One Scraper v1.0...
[CONFIG] Workers: 3 | Ports: [9050 9051 9052] | Depth: 1
[CATEGORIES] Videos | Documents | Archives | Audio | Images | Code | Executables
[LOADED] 5 target(s) to process.

[THREAD 0] Processing: example1.onion (via port 9050)
[LOAD] http://example1.onion
[DETECTED] [documents] book1.pdf
[DETECTED] [videos] lecture1.mp4
[FOUND] [archives] dataset.zip
[RESUME] Skipped 2 already downloaded files, 3 remaining
[DOCUMENT] book1.pdf: [==============================] 100.0% (5120/5120 KB)
[VIDEO] lecture1.mp4: [================>              ] 45.2% (10240/22650 KB)
[RETRY] dataset.zip: attempt 2/5
[ARCHIVE] dataset.zip: [==============================] 100.0% (2048/2048 KB)
[WARN] Failed to download old_file.pdf after retries: timeout
[SLEEP] Inter-site delay: 12m34s

[THREAD 1] Processing: example2.onion (via port 9051)
...

[SUMMARY] Downloaded: documents: 1 | videos: 1 | archives: 1
[OK] Downloaded 3 files from example1.onion

[DONE] All-in-One scraping complete!
```

**Output Tags Explained:**
- `[RESUME]` - Resuming a partial download or skipping existing files
- `[DOCUMENT]` / `[VIDEO]` / etc. - Download progress for each file type
- `[CODE]` - Source code files being downloaded
- `[WARN]` - Non-fatal warnings (timeouts, failed files)
- `[RETRY]` - Automatic retry attempts for failed downloads

---

## Automation Tips

### Linux (Cron Job)

Run the scraper on a schedule using cron:

```bash
# Edit crontab
crontab -e

# Run daily at 2 AM
0 2 * * * cd /path/to/scraper && /usr/local/go/bin/go run all_in_one_scraper.go >> /var/log/scraper.log 2>&1
```

### Windows (Task Scheduler)

1. Open **Task Scheduler** (taskschd.msc)
2. Click **Create Basic Task**
3. Set trigger (Daily/Weekly)
4. Action: **Start a program**
5. Program: `C:\Program Files\Go\bin\go.exe`
6. Arguments: `run all_in_one_scraper.go`
7. Start in: `C:\path\to\scraper`

> **Note**: This script is primarily built for Windows machines. Linux users are encouraged to adapt the code using AI code editors for cross-platform compatibility.

---

## Additional Tools

### `recursive-link-scraper.py` - Onion Indexer (Manual Download)

This is an **optional Python script** for discovering and indexing .onion URLs from paginated directory sites. It must be downloaded and used separately from the main Go scraper.

> **⚠️ NOT included in the EXE release** - Download manually from the repository or releases page.

**Features:**
- Paginated crawling (follows 'Next' page links automatically)
- Real-time SQLite database saving (prevents data loss on crashes)
- Export to text file for use with the main scraper
- Firefox 140 headers for stealth
- Configurable delays between requests

**Requirements:**
- Python 3.7+
- `requests` library: `pip install requests`
- Tor SOCKS proxy on port 9050

**Usage:**

```powershell
# Scrape a single URL with pagination
python recursive-link-scraper.py --url "http://example-index.onion" --db onion_index.db --output targets.txt

# Or process multiple URLs from a file
python recursive-link-scraper.py --file urls.txt --pages 100
```

**Typical Workflow:**
1. Download `recursive-link-scraper.py` separately (not included in EXE)
2. Use it to discover and index .onion URLs from directory sites
3. Export discovered URLs to a text file (`targets.txt`)
4. Use that file as input for the main Go scraper: `all_in_one_scraper.exe -targets targets.txt`

**Why Manual Download?**
This script is designed to be used independently based on user needs. It requires Python and different dependencies than the main Go scraper, so it's kept as a separate tool that you download only when needed.

---

## Safety & Legal Notice

This tool is intended for **educational and archival purposes only**. Always:

- Verify you have the right to download and store the content
- Respect robots.txt and site terms of service
- Be aware of local laws regarding dark web access
- Use at your own risk

---

## Requirements

- Go 1.21+
- Playwright for Go (`github.com/playwright-community/playwright-go`)
- Tor Expert Bundle configured with multiple SOCKS ports
- Windows (primary) or Linux (adaptation required)

---

## License

Use responsibly. The authors are not responsible for misuse of this tool.

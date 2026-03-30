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

### 1. Tor Expert Bundle Setup

Before running the scraper, you need to configure Tor with multiple SOCKS ports for parallel connections.

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

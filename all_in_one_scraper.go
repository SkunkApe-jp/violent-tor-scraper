package main

import (
	"bufio"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"golang.org/x/net/proxy"
)

const (
	TorProxyServer = "socks5://127.0.0.1:9050"
	TorUA          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
)

var (
	optTargetsFile string
	optOutputDir   string
	optLogFile     string
	optPorts       string
)

var (
	optInterSiteDelay int
	optIntraPageDelay int
	optWorkerCount    int
	optFastMode       bool
	optDepth          int
	optPageLoadWait   int
	optResume         bool
	optResumeFiles    bool // Skip already downloaded files
	optCrossOrigin    bool
	optNoJS           bool // Disable JavaScript
	optEnableJS       bool // Enable JavaScript (override)
)

// File type categories with their extensions
var fileCategories = map[string][]string{
	"videos": {
		".mp4", ".webm", ".mkv", ".mov", ".m4v", ".avi", ".flv", ".wmv",
		".mpg", ".mpeg", ".ogv", ".3gp", ".3g2", ".ts", ".m3u8", ".f4v",
	},
	"documents": {
		".pdf", ".epub", ".mobi", ".azw", ".azw3", ".djvu", ".djv",
		".txt", ".rtf", ".doc", ".docx", ".odt", ".chm", ".cbr", ".cbz",
		".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".md", ".tex",
	},
	"archives": {
		".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".iso",
		".tgz", ".tbz", ".txz", ".lz", ".lzma",
	},
	"audio": {
		".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".wma", ".opus",
		".aiff", ".au", ".ra", ".ram",
	},
	"images": {
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg",
		".ico", ".tiff", ".tif", ".raw", ".cr2", ".nef", ".heic",
	},
	"code": {
		".html", ".htm", ".css", ".js", ".json", ".xml", ".yaml", ".yml",
		".py", ".go", ".c", ".cpp", ".h", ".java", ".rb", ".php",
		".sh", ".bat", ".ps1", ".sql", ".log",
	},
	"executables": {
		".exe", ".msi", ".dmg", ".pkg", ".deb", ".rpm", ".appimage",
		".bin", ".run", ".sh", ".bat",
	},
}

type FileInfo struct {
	URL      string
	Category string
	Filename string
}

type ScrapedData struct {
	pageTitle string
	files     []FileInfo
}

func init() {
	flag.StringVar(&optTargetsFile, "targets", "../all_targets.yaml", "Path to targets file")
	flag.StringVar(&optOutputDir, "output", "../scraped_all", "Directory to save all downloads")
	flag.StringVar(&optLogFile, "log", "../logs/all_in_one_scraper.log", "Path to log file")
	flag.StringVar(&optPorts, "ports", "9050", "Comma-separated Tor SOCKS ports")

	flag.IntVar(&optInterSiteDelay, "inter-delay", 0, "Inter-site delay: 0=Gaussian 8-15min")
	flag.IntVar(&optIntraPageDelay, "intra-delay", 0, "Intra-page delay: 0=60-120sec")
	flag.IntVar(&optWorkerCount, "workers", 1, "Number of parallel workers")
	flag.BoolVar(&optFastMode, "fast", false, "Fast mode: reduce stealth delays")
	flag.IntVar(&optDepth, "depth", 1, "Scrape depth")
	flag.IntVar(&optPageLoadWait, "page-load-wait", 0, "Seconds to wait after page load")
	flag.BoolVar(&optResume, "resume", false, "Resume from log")
	flag.BoolVar(&optResumeFiles, "resume-files", true, "Skip already downloaded files (default: true)")
	flag.BoolVar(&optNoJS, "no-js", true, "Disable JavaScript execution (default: true for safety/speed)")
	flag.BoolVar(&optEnableJS, "js", false, "Enable JavaScript execution if needed for dynamic sites")
	flag.BoolVar(&optCrossOrigin, "cross-origin", true, "Save cross-origin files")
}

func main() {
	flag.Parse()

	ports := parsePorts(optPorts)
	if len(ports) == 0 {
		ports = []string{"9050"}
	}

	if err := os.MkdirAll(optOutputDir, 0755); err != nil {
		fmt.Printf("[ERROR] Could not create output directory: %v\n", err)
		return
	}

	// Create category subdirectories
	for category := range fileCategories {
		if err := os.MkdirAll(filepath.Join(optOutputDir, category), 0755); err != nil {
			fmt.Printf("[ERROR] Could not create %s directory: %v\n", category, err)
			return
		}
	}

	rand.Seed(time.Now().UnixNano())

	fmt.Println("[CHECK] Verifying Tor connection...")
	if !checkTorConnection() {
		fmt.Println("NOT CONNECTED TO TOR! Aborting.")
		return
	}

	workers := optWorkerCount
	if workers <= 0 {
		workers = 3
	}

	fmt.Println("[CHECK] Starting All-in-One Scraper v1.0...")
	fmt.Printf("[CONFIG] Workers: %d | Ports: %v | Depth: %d\n", workers, ports, optDepth)
	fmt.Println("[CATEGORIES] Videos | Documents | Archives | Audio | Images | Code | Executables")

	targets := loadTargets(optTargetsFile)
	if len(targets) == 0 {
		fmt.Println("No targets loaded. Exiting.")
		return
	}

	fmt.Printf("[LOADED] %d target(s) to process.\n", len(targets))

	var wg sync.WaitGroup
	targetChan := make(chan string, len(targets))

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int, port string) {
			defer wg.Done()
			workerAllInOne(id, port, targetChan)
		}(i, ports[i%len(ports)])
	}

	// Queue targets
	for _, t := range targets {
		targetChan <- t
	}
	close(targetChan)

	wg.Wait()

	fmt.Println("\n[DONE] All-in-One scraping complete!")
}

func workerAllInOne(id int, port string, targetChan <-chan string) {
	proxyServer := fmt.Sprintf("socks5://127.0.0.1:%s", port)

	for onionAddr := range targetChan {
		fmt.Printf("\n[THREAD %d] Processing: %s (via port %s)\n", id, onionAddr, port)

		data, err := processAllTypes(onionAddr, proxyServer)
		if err != nil {
			fmt.Printf("[ERROR] Failed to process %s: %v\n", onionAddr, err)
			logFailure(onionAddr, err)
			continue
		}

		if err := downloadAllFiles(onionAddr, data, proxyServer); err != nil {
			fmt.Printf("[ERROR] Failed to download files from %s: %v\n", onionAddr, err)
			logFailure(onionAddr, err)
			continue
		}

		logSuccess(onionAddr, data)
		fmt.Printf("[OK] Downloaded %d files from %s\n", len(data.files), onionAddr)

		if !optFastMode {
			delay := randomDelay(8, 15)
			fmt.Printf("[SLEEP] Inter-site delay: %v\n", delay)
			time.Sleep(delay)
		} else {
			time.Sleep(time.Duration(5+rand.Intn(10)) * time.Second)
		}
	}
}

func processAllTypes(onionAddr string, proxyServer string) (*ScrapedData, error) {
	data := &ScrapedData{
		files: []FileInfo{},
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, err
	}
	defer pw.Stop()

	launchOptions := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--proxy-server=" + proxyServer,
			"--disable-blink-features=AutomationControlled",
			"--disable-web-security",
			"--disable-features=IsolateOrigins,site-per-process",
		},
	}

	if optNoJS && !optEnableJS {
		launchOptions.JavaScriptEnabled = playwright.Bool(false)
	}

	context, err := pw.Chromium.LaunchPersistentContext(
		getTorProfilePath(),
		launchOptions,
	)
	if err != nil {
		return nil, err
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return nil, err
	}
	page.SetDefaultTimeout(300000)

	var resMu sync.Mutex
	capturedFiles := make(map[string]bool)

	page.On("response", func(response playwright.Response) {
		resURL := response.URL()
		lowerURL := strings.ToLower(resURL)

		// Skip already captured
		resMu.Lock()
		if capturedFiles[resURL] {
			resMu.Unlock()
			return
		}
		resMu.Unlock()

		// Check cross-origin
		if !optCrossOrigin && !strings.Contains(resURL, onionAddr) {
			return
		}

		// Determine category
		category := detectCategory(lowerURL)
		if category == "" {
			return // Not a file we want to download
		}

		resMu.Lock()
		capturedFiles[resURL] = true
		resMu.Unlock()

		// Get filename
		filename := generateFilename(resURL, category)

		info := FileInfo{
			URL:      resURL,
			Category: category,
			Filename: filename,
		}

		data.files = append(data.files, info)
		fmt.Printf("[DETECTED] [%s] %s\n", category, filepath.Base(filename))
	})

	pageLoadWait := optPageLoadWait
	if pageLoadWait == 0 {
		if optFastMode {
			pageLoadWait = 8
		} else {
			pageLoadWait = 45
		}
	}

	targetURL := onionAddr
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}
	fmt.Printf("[LOAD] %s\n", targetURL)

	if _, err := page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(120000),
	}); err != nil {
		return nil, fmt.Errorf("page load failed: %v", err)
	}

	title, _ := page.Title()
	data.pageTitle = title

	time.Sleep(time.Duration(pageLoadWait) * time.Second)

	// Extract file links from <a> tags
	extractFileLinks(page, onionAddr, data, capturedFiles, &resMu)

	if optDepth > 1 {
		links, _ := page.Locator("a").All()
		for i, link := range links {
			if i >= 20 {
				break
			}
			href, _ := link.GetAttribute("href")
			if href != "" && !strings.HasPrefix(href, "javascript:") {
				link.Click()
				time.Sleep(3 * time.Second)
			}
		}
	}

	return data, nil
}

func detectCategory(lowerURL string) string {
	// Remove query parameters and fragments for extension detection
	cleanURL := lowerURL
	if idx := strings.Index(cleanURL, "?"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}
	if idx := strings.Index(cleanURL, "#"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}

	for category, extensions := range fileCategories {
		for _, ext := range extensions {
			if strings.HasSuffix(cleanURL, ext) {
				return category
			}
		}
	}
	return ""
}

func generateFilename(resURL, category string) string {
	u, err := url.Parse(resURL)
	if err != nil {
		hash := sha256.Sum256([]byte(resURL))
		return fmt.Sprintf("%s_%s_%x", category, "unknown", hash[:8])
	}

	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		hash := sha256.Sum256([]byte(resURL))
		base = fmt.Sprintf("%x_file", hash[:8])
	}

	// Clean filename
	clean := strings.Map(func(r rune) rune {
		if r == '<' || r == '>' || r == ':' || r == '"' || r == '/' ||
			r == '\\' || r == '|' || r == '?' || r == '*' || r == '%' {
			return '_'
		}
		return r
	}, base)

	if len(clean) > 200 {
		clean = clean[:200]
	}

	return clean
}

func downloadAllFiles(onionAddr string, data *ScrapedData, proxyServer string) error {
	if len(data.files) == 0 {
		return nil
	}

	// Extract clean domain for folder structure
	domainFolder := extractDomainForFolder(onionAddr)

	// Filter out already downloaded files (resume functionality)
	var filesToDownload []FileInfo
	skippedCount := 0
	
	if optResumeFiles {
		for _, file := range data.files {
			baseDir := filepath.Join(optOutputDir, file.Category, domainFolder)
			fullPath := windowsFriendlyPath(filepath.Join(baseDir, file.Filename))
			
			// Check if file exists and is complete
			if info, err := os.Stat(fullPath); err == nil && info.Size() > 0 {
				// File exists - skip it
				fmt.Printf("[SKIP] [%s] %s (already exists, %d KB)\n", file.Category, filepath.Base(file.Filename), info.Size()/1024)
				skippedCount++
				continue
			}
			filesToDownload = append(filesToDownload, file)
		}
		
		if skippedCount > 0 {
			fmt.Printf("[RESUME] Skipped %d already downloaded files, %d remaining\n", skippedCount, len(filesToDownload))
		}
		
		if len(filesToDownload) == 0 {
			fmt.Println("[RESUME] All files already downloaded!")
			return nil
		}
	} else {
		filesToDownload = data.files
	}

	// Setup Tor Proxy Client
	proxyURL, err := url.Parse(proxyServer)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %v", err)
	}
	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return fmt.Errorf("could not create proxy dialer: %v", err)
	}

	transport := &http.Transport{
		Dial:                dialer.Dial,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Minute,
	}

	// Group files by category for summary
	categoryCounts := make(map[string]int)

	for _, file := range filesToDownload {
		baseDir := filepath.Join(optOutputDir, file.Category, domainFolder)
		fullPath := windowsFriendlyPath(filepath.Join(baseDir, file.Filename))

		if err := os.MkdirAll(baseDir, 0755); err != nil {
			fmt.Printf("[WARN] Failed to create directory %s: %v\n", baseDir, err)
			continue
		}

		// Download with retries
		err := downloadFileWithRetry(client, file, fullPath, 5000)
		if err != nil {
			fmt.Printf("[WARN] Failed to download %s after retries: %v\n", file.URL, err)
			continue
		}

		categoryCounts[file.Category]++
	}

	// Print summary
	fmt.Printf("[SUMMARY] Downloaded: ")
	first := true
	for cat, count := range categoryCounts {
		if !first {
			fmt.Print(" | ")
		}
		fmt.Printf("%s: %d", cat, count)
		first = false
	}
	fmt.Println()

	return nil
}

type ProgressWriter struct {
	Total      int64
	Downloaded int64
	Filename   string
	Label      string
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)
	pw.printProgress()
	return n, nil
}

func (pw *ProgressWriter) printProgress() {
	if pw.Total <= 0 {
		fmt.Printf("\r[%s] %s: %d KB downloaded...", pw.Label, pw.Filename, pw.Downloaded/1024)
		return
	}
	percent := float64(pw.Downloaded) / float64(pw.Total) * 100
	width := 25
	filled := int(float64(width) * float64(pw.Downloaded) / float64(pw.Total))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	fmt.Printf("\r[%s] %s: [%s] %.1f%% (%d/%d KB)", pw.Label, pw.Filename, bar, percent, pw.Downloaded/1024, pw.Total/1024)
}

func windowsFriendlyPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	// Convert to absolute path first - \\?\ requires absolute paths
	absPath, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return "\\\\?\\" + absPath
}

func parsePorts(portsStr string) []string {
	if portsStr == "" {
		return []string{"9050"}
	}
	parts := strings.Split(portsStr, ",")
	var ports []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			ports = append(ports, p)
		}
	}
	return ports
}

func checkTorConnection() bool {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				dialer, _ := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
				return dialer.Dial(network, addr)
			},
		},
	}

	resp, err := client.Get("http://check.torproject.org")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(body), "Congratulations")
}

func loadTargets(filename string) []string {
	var targets []string
	file, err := os.Open(filename)
	if err != nil {
		return targets
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			targets = append(targets, line)
		}
	}
	return targets
}

func getTorProfilePath() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "playwright_tor_profile")
}

func logSuccess(onionAddr string, data *ScrapedData) {
	f, err := os.OpenFile(optLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)
	logger.Printf("[SUCCESS] %s - %d files - %s", onionAddr, len(data.files), data.pageTitle)
}

func logFailure(onionAddr string, err error) {
	f, err := os.OpenFile(optLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)
	logger.Printf("[FAILED] %s - %v", onionAddr, err)
}

func randomDelay(min, max int) time.Duration {
	mean := float64(min+max) / 2
	stddev := float64(max-min) / 6
	delay := rand.NormFloat64()*stddev + mean
	if delay < float64(min) {
		delay = float64(min)
	}
	if delay > float64(max) {
		delay = float64(max)
	}
	return time.Duration(delay * float64(time.Minute))
}
// downloadFileWithRetry downloads a file with retry logic and resume support
func downloadFileWithRetry(client *http.Client, file FileInfo, fullPath string, maxRetries int) error {
	const TorUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	label := strings.ToUpper(file.Category)
	filename := filepath.Base(file.Filename)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("[RETRY] %s: attempt %d/%d\n", filename, attempt+1, maxRetries)
			time.Sleep(2 * time.Second) // Short, consistent delay
		}

		// Check existing file size for resume
		var startOffset int64 = 0
		if info, err := os.Stat(fullPath); err == nil {
			startOffset = info.Size()
			if startOffset > 0 {
				fmt.Printf("[RESUME] %s: resuming from %d KB\n", filename, startOffset/1024)
			}
		}

		req, err := http.NewRequest("GET", file.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", TorUA)
		if startOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[WARN] Connection error: %v\n", err)
			continue
		}

		// If server doesn't support resume, restart from beginning
		if startOffset > 0 && resp.StatusCode != http.StatusPartialContent {
			startOffset = 0
			os.Remove(fullPath) // Remove partial file
			resp.Body.Close()
			continue // Retry from beginning
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return fmt.Errorf("non-200 status: %s", resp.Status)
		}

		// Open file for writing (append if resuming)
		flag := os.O_CREATE | os.O_WRONLY
		if startOffset > 0 {
			flag = os.O_APPEND | os.O_WRONLY
		}
		out, err := os.OpenFile(fullPath, flag, 0644)
		if err != nil {
			resp.Body.Close()
			return err
		}

		pw := &ProgressWriter{
			Total:       resp.ContentLength + startOffset,
			Downloaded:  startOffset,
			Filename:    filename,
			Label:       label,
		}

		_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
		out.Close()
		resp.Body.Close()
		fmt.Println()

		if err != nil {
			fmt.Printf("[WARN] Download interrupted: %v\n", err)
			continue // Retry
		}

		return nil // Success
	}

	return fmt.Errorf("failed after %d attempts", maxRetries)
}
func extractDomainForFolder(urlStr string) string {
	// If it has a protocol, parse it
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		u, err := url.Parse(urlStr)
		if err == nil && u.Host != "" {
			return strings.TrimPrefix(u.Host, "www.")
		}
	}
	// Otherwise assume it's already just a domain/path
	return strings.TrimPrefix(urlStr, "www.")
}

// extractFileLinks scans all <a> tags and extracts links to downloadable files
func extractFileLinks(page playwright.Page, baseURL string, data *ScrapedData, capturedFiles map[string]bool, resMu *sync.Mutex) {
	links, err := page.Locator("a").All()
	if err != nil {
		return
	}

	baseParsed, _ := url.Parse(baseURL)
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseParsed, _ = url.Parse("http://" + baseURL)
	}

	for _, link := range links {
		href, err := link.GetAttribute("href")
		if err != nil || href == "" {
			continue
		}

		// Skip javascript and anchors
		if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "#") {
			continue
		}

		// Resolve relative URLs
		var fullURL string
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			fullURL = href
		} else if baseParsed != nil {
			rel, err := url.Parse(href)
			if err != nil {
				continue
			}
			resolved := baseParsed.ResolveReference(rel)
			fullURL = resolved.String()
		} else {
			fullURL = href
		}

		lowerURL := strings.ToLower(fullURL)

		// Check if already captured
		resMu.Lock()
		if capturedFiles[fullURL] {
			resMu.Unlock()
			continue
		}

		// Check if it's a file we want
		category := detectCategory(lowerURL)
		if category == "" {
			resMu.Unlock()
			continue
		}

		capturedFiles[fullURL] = true
		resMu.Unlock()

		filename := generateFilename(fullURL, category)
		info := FileInfo{
			URL:      fullURL,
			Category: category,
			Filename: filename,
		}

		data.files = append(data.files, info)
		fmt.Printf("[FOUND] [%s] %s\n", category, filepath.Base(filename))
	}
}

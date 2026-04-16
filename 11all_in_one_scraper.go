package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	optStayUnderPath  bool // Only crawl under starting path
	optMaxPages       int  // Max pages per target
	optMaxFiles       int  // Max files to capture per target
	optMaxBytesTarget int64
	optMaxBytesFile   int64
	optDLRetries      int
	optDLWorkers      int
	optAllowPathRegex string
	optDenyPathRegex  string
	optDenyQueryKeys  string
	optMaxRuntimeSecs int
	optBlockResources bool
	optDumpOnFail     bool
	optDLPreflight    bool
)

var (
	allowPathRe   *regexp.Regexp
	denyPathRe    *regexp.Regexp
	denyQueryKeys map[string]bool
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

type DownloadedMeta struct {
	FinalURL    string
	ContentType string
	Size        int64
	SHA256      string
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
	flag.BoolVar(&optStayUnderPath, "stay-under-path", true, "Only crawl URLs under the starting path")
	flag.IntVar(&optMaxPages, "max-pages", 2000, "Maximum pages to crawl per target")
	flag.IntVar(&optMaxFiles, "max-files", 20000, "Maximum files to capture per target")
	flag.Int64Var(&optMaxBytesTarget, "max-bytes-target", 0, "Maximum bytes to download per target (0 = unlimited)")
	flag.Int64Var(&optMaxBytesFile, "max-bytes-file", 0, "Maximum bytes to download per file (0 = unlimited)")
	flag.IntVar(&optDLRetries, "dl-retries", 12, "Download retries per file")
	flag.IntVar(&optDLWorkers, "dl-workers", 1, "Parallel downloads per target")
	flag.StringVar(&optAllowPathRegex, "allow-path-regex", "", "Only crawl URLs whose path matches this regex (empty = allow all)")
	flag.StringVar(&optDenyPathRegex, "deny-path-regex", "", "Do not crawl URLs whose path matches this regex (empty = deny none)")
	flag.StringVar(&optDenyQueryKeys, "deny-query-keys", "", "Comma-separated query keys to skip crawling when present")
	flag.IntVar(&optMaxRuntimeSecs, "max-runtime-target", 0, "Max runtime per target in seconds (0 = unlimited)")
	flag.BoolVar(&optBlockResources, "block-resources", true, "Block images/fonts/media to reduce Tor load")
	flag.BoolVar(&optDumpOnFail, "dump-on-fail", true, "Save screenshot+HTML on navigation failures")
	flag.BoolVar(&optDLPreflight, "dl-preflight", false, "Preflight downloads with HEAD/GET to validate content-length/type when possible")
}

func main() {
	flag.Parse()

	var err error
	if optAllowPathRegex != "" {
		allowPathRe, err = regexp.Compile(optAllowPathRegex)
		if err != nil {
			fmt.Printf("[ERROR] invalid --allow-path-regex: %v\n", err)
			return
		}
	}
	if optDenyPathRegex != "" {
		denyPathRe, err = regexp.Compile(optDenyPathRegex)
		if err != nil {
			fmt.Printf("[ERROR] invalid --deny-path-regex: %v\n", err)
			return
		}
	}
	denyQueryKeys = parseCSVSet(optDenyQueryKeys)

	ports := parsePorts(optPorts)
	if len(ports) == 0 {
		ports = []string{"9050"}
	}

	if err := os.MkdirAll(optOutputDir, 0755); err != nil {
		fmt.Printf("[ERROR] Could not create output directory: %v\n", err)
		return
	}

	// Skip category subdirectory creation - files go directly under domain folders
	// Subfolders are created per URL path as needed during download

	rand.Seed(time.Now().UnixNano())

	fmt.Println("[CHECK] Verifying Tor connection...")
	if !checkTorConnection(ports) {
		fmt.Println("NOT CONNECTED TO TOR! Aborting.")
		return
	}

	workers := optWorkerCount
	if workers <= 0 {
		workers = 3
	}
	if optDLWorkers <= 0 {
		optDLWorkers = 1
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

	var processedCount int32
	var failedCount int32
	var completedCount int32
	totalTargets := int32(len(targets))

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int, port string) {
			defer wg.Done()
			workerAllInOne(id, port, targetChan, &processedCount, &failedCount, &completedCount, totalTargets)
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

func workerAllInOne(id int, port string, targetChan <-chan string, processedCount, failedCount, completedCount *int32, totalTargets int32) {
	proxyServer := fmt.Sprintf("socks5://127.0.0.1:%s", port)

	pw, context, err := setupWorkerBrowser(proxyServer, id)
	if err != nil {
		fmt.Printf("[ERROR] Worker %d failed to start browser: %v\n", id, err)
		return
	}
	defer func() {
		if context != nil {
			_ = context.Close()
		}
		if pw != nil {
			pw.Stop()
		}
	}()

	for onionAddr := range targetChan {
		atomic.AddInt32(processedCount, 1)

		fmt.Printf("\n[THREAD %d] ..... %s (via port %s)\n", id, onionAddr, port)

		data, err := processAllTypes(context, onionAddr, proxyServer)
		if err != nil {
			fmt.Printf("[ERROR] Failed to process %s: %v\n", onionAddr, err)
			logFailure(onionAddr, err)
			atomic.AddInt32(failedCount, 1)
			continue
		}

		bytesDownloaded, dlMeta, err := downloadAllFiles(onionAddr, data, proxyServer)
		if err != nil {
			fmt.Printf("[ERROR] Failed to download files from %s: %v\n", onionAddr, err)
			logFailure(onionAddr, err)
			atomic.AddInt32(failedCount, 1)
			continue
		}

		atomic.AddInt32(completedCount, 1)
		logSuccess(onionAddr, data, bytesDownloaded, dlMeta)
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

func setupWorkerBrowser(proxyServer string, workerID int) (*playwright.Playwright, playwright.BrowserContext, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, nil, err
	}

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
		getTorProfilePath(workerID),
		launchOptions,
	)
	if err != nil {
		pw.Stop()
		return nil, nil, err
	}

	return pw, context, nil
}

func processAllTypes(context playwright.BrowserContext, onionAddr string, proxyServer string) (*ScrapedData, error) {
	data := &ScrapedData{
		files: []FileInfo{},
	}

	page, err := context.NewPage()
	if err != nil {
		return nil, err
	}
	defer page.Close()

	if optBlockResources {
		_ = page.Route("**/*", func(route playwright.Route) {
			request := route.Request()
			switch request.ResourceType() {
			case "image", "media", "font":
				_ = route.Abort()
				return
			}
			_ = route.Continue()
		})
	}
	page.SetDefaultTimeout(300000)

	var resMu sync.Mutex
	capturedFiles := make(map[string]bool)

	page.On("response", func(response playwright.Response) {
		resURL := response.URL()
		lowerURL := strings.ToLower(resURL)

		resMu.Lock()
		if capturedFiles[resURL] {
			resMu.Unlock()
			return
		}
		resMu.Unlock()

		if !optCrossOrigin && !strings.Contains(resURL, onionAddr) {
			return
		}

		category := detectCategory(lowerURL)
		if category == "" {
			return
		}

		resMu.Lock()
		if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
			resMu.Unlock()
			return
		}
		capturedFiles[resURL] = true
		resMu.Unlock()

		filename := generateFilename(resURL, category)

		info := FileInfo{
			URL:      resURL,
			Category: category,
			Filename: filename,
		}

		resMu.Lock()
		if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
			resMu.Unlock()
			return
		}
		data.files = append(data.files, info)
		resMu.Unlock()
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

	startTime := time.Now()
	maxRuntime := time.Duration(optMaxRuntimeSecs) * time.Second

	// Parse base domain and path for same-origin and path filtering
	baseParsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %v", err)
	}
	baseHost := canonicalHost(baseParsed)
	basePath := baseParsed.Path
	
	fmt.Printf("[DEBUG] Starting crawl with basePath=%s depth=%d max-pages=%d\n", basePath, optDepth, optMaxPages)
	
	pageCount := 0

	fmt.Printf("[LOAD] %s\n", targetURL)

	type crawlItem struct {
		URL   string
		Depth int
	}

	// Crawl subdirectories with BFS (depth per URL item)
	visited := make(map[string]bool)
	startCanon, err := canonicalizeForVisit(targetURL, denyQueryKeys)
	if err != nil {
		return nil, err
	}
	queue := []crawlItem{{URL: startCanon, Depth: 0}}
	visited[startCanon] = true

	for len(queue) > 0 {
		if maxRuntime > 0 && time.Since(startTime) > maxRuntime {
			fmt.Printf("[LIMIT] Reached max-runtime-target (%v), stopping crawl\n", maxRuntime)
			break
		}

		item := queue[0]
		queue = queue[1:]
		if item.Depth >= optDepth {
			continue
		}

		if _, err := page.Goto(item.URL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateNetworkidle,
			Timeout:   playwright.Float(120000),
		}); err != nil {
			fmt.Printf("[WARN] Failed to load %s: %v\n", item.URL, err)
			if optDumpOnFail {
				_ = dumpPageFailure(onionAddr, page, "goto")
			}
			continue
		}

		title, _ := page.Title()
		if item.Depth == 0 {
			data.pageTitle = title
		}

		time.Sleep(time.Duration(pageLoadWait) * time.Second)

		// Extract file links from current page
		extractFileLinks(page, item.URL, data, capturedFiles, &resMu)

		// Find subdirectory links for next depth level
		if item.Depth+1 < optDepth {
			links, _ := page.Locator("a").All()
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
				} else {
					currentParsed, _ := url.Parse(item.URL)
					if currentParsed != nil {
						rel, err := url.Parse(href)
						if err != nil {
							continue
						}
						resolved := currentParsed.ResolveReference(rel)
						fullURL = resolved.String()
					} else {
						fullURL = href
					}
				}

				canon, err := canonicalizeForVisit(fullURL, denyQueryKeys)
				if err != nil {
					continue
				}

				linkParsed, err := url.Parse(canon)
				if err != nil || canonicalHost(linkParsed) != baseHost {
					fmt.Printf("[DEBUG] Rejected (wrong host): %s\n", canon)
					continue
				}

				// Check path constraint if enabled
				if optStayUnderPath {
					linkPath := linkParsed.Path
					if !strings.HasPrefix(linkPath, basePath) {
						fmt.Printf("[DEBUG] Rejected (outside path): linkPath=%s basePath=%s\n", linkPath, basePath)
						continue
					}
				}

				if allowPathRe != nil && !allowPathRe.MatchString(linkParsed.Path) {
					continue
				}
				if denyPathRe != nil && denyPathRe.MatchString(linkParsed.Path) {
					continue
				}
				if hasAnyQueryKey(fullURL, denyQueryKeys) {
					continue
				}

				// Check max pages limit
				if pageCount >= optMaxPages {
					fmt.Printf("[LIMIT] Reached max-pages (%d), stopping crawl\n", optMaxPages)
					break
				}

				// Skip if already visited or queued
				if visited[canon] {
					fmt.Printf("[DEBUG] Rejected (already visited): %s (from: %s)\n", canon, fullURL)
					continue
				}

				// Only follow if it's a directory-like URL (ends with / or has path structure)
				// or if it's not a file URL
				lowerURL := strings.ToLower(canon)
				if detectCategory(lowerURL) != "" {
					// It's a file URL, don't crawl it as a page, but do capture it for download.
					captureFileURL(canon, data, capturedFiles, &resMu)
					continue
				}

				visited[canon] = true
				queue = append(queue, crawlItem{URL: canon, Depth: item.Depth + 1})
				fmt.Printf("[QUEUE] Found subdirectory: %s (from href: %s)\n", canon, href)
			}
		}

		pageCount++
		if pageCount >= optMaxPages {
			fmt.Printf("[LIMIT] Reached max-pages (%d), stopping crawl\n", optMaxPages)
			break
		}

		if !optFastMode && item.Depth+1 < optDepth {
			time.Sleep(time.Duration(3+rand.Intn(5)) * time.Second)
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

	// Get the directory path and filename
	dir := filepath.Dir(u.Path)
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

	// Clean the directory path components
	if dir != "" && dir != "." && dir != "/" {
		// Remove leading slash and clean each component
		dir = strings.TrimPrefix(dir, "/")
		components := strings.Split(dir, "/")
		var cleanedComponents []string
		for _, comp := range components {
			if comp == "" || comp == "." {
				continue
			}
			// Clean component of invalid characters
			cleanComp := strings.Map(func(r rune) rune {
				if r == '<' || r == '>' || r == ':' || r == '"' || r == '/' ||
					r == '\\' || r == '|' || r == '?' || r == '*' || r == '%' {
					return '_'
				}
				return r
			}, comp)
			if cleanComp != "" {
				cleanedComponents = append(cleanedComponents, cleanComp)
			}
		}
		if len(cleanedComponents) > 0 {
			return filepath.Join(append(cleanedComponents, clean)...)
		}
	}

	return clean
}

func downloadAllFiles(onionAddr string, data *ScrapedData, proxyServer string) (int64, map[string]DownloadedMeta, error) {
	if len(data.files) == 0 {
		return 0, map[string]DownloadedMeta{}, nil
	}

	// Extract clean domain for folder structure
	domainFolder := extractDomainForFolder(onionAddr)

	// Filter out already downloaded files (resume functionality)
	var filesToDownload []FileInfo
	skippedCount := 0
	
	if optResumeFiles {
		for _, file := range data.files {
			// New structure: optOutputDir/domainFolder/subpath/filename (no category folder)
			baseDir := filepath.Join(optOutputDir, domainFolder)
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
			return 0, map[string]DownloadedMeta{}, nil
		}
	} else {
		filesToDownload = data.files
	}

	// Setup Tor Proxy Client
	proxyURL, err := url.Parse(proxyServer)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid proxy URL: %v", err)
	}
	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return 0, nil, fmt.Errorf("could not create proxy dialer: %v", err)
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

	var bytesDownloaded int64
	startTime := time.Now()
	maxRuntime := time.Duration(optMaxRuntimeSecs) * time.Second
	var fileCounter int32

	// Group files by category for summary
	var catMu sync.Mutex
	categoryCounts := make(map[string]int)
	metaMu := sync.Mutex{}
	downloadMeta := make(map[string]DownloadedMeta)

	totalFilesToDownload := len(filesToDownload)
	jobs := make(chan FileInfo)
	var wg sync.WaitGroup

	workerCount := optDLWorkers
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > totalFilesToDownload {
		workerCount = totalFilesToDownload
	}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for file := range jobs {
				idx := int(atomic.AddInt32(&fileCounter, 1))
				if maxRuntime > 0 && time.Since(startTime) > maxRuntime {
					continue
				}
				if optMaxBytesTarget > 0 {
					current := atomic.LoadInt64(&bytesDownloaded)
					if current >= optMaxBytesTarget {
						fmt.Printf("[LIMIT] Reached max-bytes-target (%d), skipping remaining downloads\n", optMaxBytesTarget)
						continue
					}
				}

				baseDir := filepath.Join(optOutputDir, domainFolder)
				fullPath := windowsFriendlyPath(filepath.Join(baseDir, file.Filename))

				dirPath := filepath.Dir(fullPath)
				if err := os.MkdirAll(dirPath, 0755); err != nil {
					fmt.Printf("[WARN] Failed to create directory %s: %v\n", dirPath, err)
					continue
				}

				remainingBudget := int64(0)
				if optMaxBytesTarget > 0 {
					current := atomic.LoadInt64(&bytesDownloaded)
					remainingBudget = optMaxBytesTarget - current
					if remainingBudget <= 0 {
						fmt.Printf("[LIMIT] Reached max-bytes-target (%d), skipping remaining downloads\n", optMaxBytesTarget)
						continue
					}
				}

				written, meta, err := downloadFileWithRetry(client, file, fullPath, optDLRetries, idx, totalFilesToDownload, remainingBudget)
				if err != nil {
					fmt.Printf("[WARN] Failed to download %s after retries: %v\n", file.URL, err)
					continue
				}

				if written > 0 {
					atomic.AddInt64(&bytesDownloaded, written)
				}

				catMu.Lock()
				categoryCounts[file.Category]++
				catMu.Unlock()

				if meta != nil {
					metaMu.Lock()
					downloadMeta[file.URL] = *meta
					metaMu.Unlock()
				}
			}
		}(w)
	}

	for _, file := range filesToDownload {
		jobs <- file
	}
	close(jobs)
	
	wg.Wait()

	// Print summary
	fmt.Printf("[SUMMARY] Downloaded: ")
	cats := make([]string, 0, len(categoryCounts))
	for cat := range categoryCounts {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	for i, cat := range cats {
		if i > 0 {
			fmt.Print(" | ")
		}
		fmt.Printf("%s: %d", cat, categoryCounts[cat])
	}
	fmt.Println()

	if optMaxBytesTarget > 0 {
		fmt.Printf("[SUMMARY] Bytes downloaded (approx): %d\n", atomic.LoadInt64(&bytesDownloaded))
	}

	return atomic.LoadInt64(&bytesDownloaded), downloadMeta, nil
}

type ProgressWriter struct {
	Total      int64
	Downloaded int64
	Filename   string
	Label      string
	FileIndex  int
	TotalFiles int
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)
	pw.printProgress()
	return n, nil
}

func (pw *ProgressWriter) printProgress() {
	if pw.Total <= 0 {
		fmt.Printf("\r[%s] [File %d/%d] %s: %d KB downloaded...          ", pw.Label, pw.FileIndex, pw.TotalFiles, pw.Filename, pw.Downloaded/1024)
		return
	}
	percent := float64(pw.Downloaded) / float64(pw.Total) * 100
	width := 25
	filled := int(float64(width) * float64(pw.Downloaded) / float64(pw.Total))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	fmt.Printf("\r[%s] [File %d/%d] %s: [%s] %.1f%% (%d/%d KB)          ", pw.Label, pw.FileIndex, pw.TotalFiles, pw.Filename, bar, percent, pw.Downloaded/1024, pw.Total/1024)
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

func checkTorConnection(ports []string) bool {
	if len(ports) == 0 {
		ports = []string{"9050"}
	}

	for _, port := range ports {
		client := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Dial: func(network, addr string) (net.Conn, error) {
					dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+port, nil, proxy.Direct)
					if err != nil {
						return nil, err
					}
					return dialer.Dial(network, addr)
				},
			},
		}

		resp, err := client.Get("http://check.torproject.org")
		if err != nil {
			fmt.Printf("[CHECK] Tor check failed via port %s: %v\n", port, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "Congratulations") {
			fmt.Printf("[CHECK] Tor OK via port %s\n", port)
			return true
		}
		fmt.Printf("[CHECK] Tor check did not confirm Tor via port %s\n", port)
	}

	return false
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


func getTorProfilePath(workerID int) string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, fmt.Sprintf("playwright_tor_profile_w%d", workerID))
}

func saveFileMetadata(onionAddr string, data *ScrapedData, dlMeta map[string]DownloadedMeta) error {
	// Extract domain for folder structure
	domainFolder := extractDomainForFolder(onionAddr)
	
	// Create metadata directory
	metaDir := filepath.Join(optOutputDir, "metadata")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return err
	}
	
	// Clean domain name for filename
	safeDomain := strings.ReplaceAll(domainFolder, ":", "_")
	safeDomain = strings.ReplaceAll(safeDomain, "/", "_")
	metaFile := filepath.Join(metaDir, safeDomain+"_files.json")
	
	// Build metadata structure
	type FileMeta struct {
		URL         string `json:"url"`
		Category    string `json:"category"`
		Filename    string `json:"filename"`
		RelPath     string `json:"rel_path"`
		FinalURL    string `json:"final_url,omitempty"`
		ContentType string `json:"content_type,omitempty"`
		Size        int64  `json:"size,omitempty"`
		SHA256      string `json:"sha256,omitempty"`
	}
	
	var metadata []FileMeta
	for _, file := range data.files {
		// New structure: domainFolder/subpath/filename (no category folder)
		relPath := filepath.Join(domainFolder, file.Filename)
		dm := dlMeta[file.URL]
		metadata = append(metadata, FileMeta{
			URL:         file.URL,
			Category:    file.Category,
			Filename:    file.Filename,
			RelPath:     relPath,
			FinalURL:    dm.FinalURL,
			ContentType: dm.ContentType,
			Size:        dm.Size,
			SHA256:      dm.SHA256,
		})
	}
	
	// Write JSON
	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(metaFile, jsonData, 0644)
}

func logSuccess(onionAddr string, data *ScrapedData, bytesDownloaded int64, dlMeta map[string]DownloadedMeta) {
	// Also save detailed metadata for file organization
	if err := saveFileMetadata(onionAddr, data, dlMeta); err != nil {
		fmt.Printf("[WARN] Failed to save metadata for %s: %v\n", onionAddr, err)
	}
	if err := saveTargetSummary(onionAddr, data, bytesDownloaded); err != nil {
		fmt.Printf("[WARN] Failed to save summary for %s: %v\n", onionAddr, err)
	}
	
	f, err := os.OpenFile(optLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)
	logger.Printf("[SUCCESS] %s - %d files - %d bytes - %s", onionAddr, len(data.files), bytesDownloaded, data.pageTitle)
}

func saveTargetSummary(onionAddr string, data *ScrapedData, bytesDownloaded int64) error {
	domainFolder := extractDomainForFolder(onionAddr)
	metaDir := filepath.Join(optOutputDir, "metadata")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return err
	}
	safeDomain := strings.ReplaceAll(domainFolder, ":", "_")
	safeDomain = strings.ReplaceAll(safeDomain, "/", "_")
	summaryFile := filepath.Join(metaDir, safeDomain+"_summary.json")

	type Summary struct {
		Target         string `json:"target"`
		Title          string `json:"title"`
		FilesDetected  int    `json:"files_detected"`
		BytesDownloaded int64 `json:"bytes_downloaded"`
	}

	s := Summary{
		Target:         onionAddr,
		Title:          data.pageTitle,
		FilesDetected:  len(data.files),
		BytesDownloaded: bytesDownloaded,
	}

	jsonData, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(summaryFile, jsonData, 0644)
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
func downloadFileWithRetry(client *http.Client, file FileInfo, fullPath string, maxRetries int, fileIndex int, totalFiles int, remainingTargetBudget int64) (int64, *DownloadedMeta, error) {
	const TorUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	label := strings.ToUpper(file.Category)
	filename := filepath.Base(file.Filename)
	partPath := fullPath + ".part"

	if maxRetries <= 0 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("[RETRY] %s: attempt %d/%d\n", filename, attempt+1, maxRetries)
			sleep := time.Duration(1<<min(attempt, 6)) * time.Second
			if sleep > 60*time.Second {
				sleep = 60 * time.Second
			}
			time.Sleep(sleep)
		}

		// Check existing file size for resume
		var startOffset int64 = 0
		if info, err := os.Stat(partPath); err == nil {
			startOffset = info.Size()
			if startOffset > 0 {
				fmt.Printf("[RESUME] %s: resuming from %d KB\n", filename, startOffset/1024)
			}
		}

		if optDLPreflight {
			ct, cl, finalURL, ok := preflightDownload(client, file.URL)
			if ok {
				if optMaxBytesFile > 0 && cl > 0 && cl+startOffset > optMaxBytesFile {
					return 0, nil, fmt.Errorf("file exceeds max-bytes-file (%d)", optMaxBytesFile)
				}
				if remainingTargetBudget > 0 && cl > 0 && cl+startOffset > remainingTargetBudget {
					return 0, nil, fmt.Errorf("file exceeds remaining max-bytes-target budget (%d)", remainingTargetBudget)
				}
				if shouldRejectContentType(file.Category, ct) {
					return 0, nil, fmt.Errorf("rejected by content-type preflight: %s", ct)
				}
				_ = finalURL
			}
		}

		req, err := http.NewRequest("GET", file.URL, nil)
		if err != nil {
			return 0, nil, err
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
			os.Remove(partPath)
			resp.Body.Close()
			continue // Retry from beginning
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return 0, nil, fmt.Errorf("non-200 status: %s", resp.Status)
		}
		if shouldRejectContentType(file.Category, resp.Header.Get("Content-Type")) {
			resp.Body.Close()
			return 0, nil, fmt.Errorf("rejected by content-type: %s", resp.Header.Get("Content-Type"))
		}

		if optMaxBytesFile > 0 && resp.ContentLength > 0 && resp.ContentLength+startOffset > optMaxBytesFile {
			resp.Body.Close()
			return 0, nil, fmt.Errorf("file exceeds max-bytes-file (%d)", optMaxBytesFile)
		}
		if remainingTargetBudget > 0 && resp.ContentLength > 0 && resp.ContentLength+startOffset > remainingTargetBudget {
			resp.Body.Close()
			return 0, nil, fmt.Errorf("file exceeds remaining max-bytes-target budget (%d)", remainingTargetBudget)
		}

		// Open file for writing (append if resuming)
		flag := os.O_CREATE | os.O_WRONLY
		if startOffset > 0 {
			flag = os.O_APPEND | os.O_WRONLY
		}
		out, err := os.OpenFile(partPath, flag, 0644)
		if err != nil {
			resp.Body.Close()
			return 0, nil, err
		}

		pw := &ProgressWriter{
			Total:       resp.ContentLength + startOffset,
			Downloaded:  startOffset,
			Filename:    filename,
			Label:       label,
			FileIndex:   fileIndex,
			TotalFiles:  totalFiles,
		}

		reader := io.Reader(resp.Body)
		if optMaxBytesFile > 0 {
			reader = io.LimitReader(reader, optMaxBytesFile-startOffset+1)
		}
		if remainingTargetBudget > 0 {
			reader = io.LimitReader(reader, remainingTargetBudget-startOffset+1)
		}

		h := sha256.New()
		mw := io.MultiWriter(pw, h)
		written, err := io.Copy(out, io.TeeReader(reader, mw))
		out.Close()
		resp.Body.Close()
		fmt.Println()

		if err != nil {
			fmt.Printf("[WARN] Download interrupted: %v\n", err)
			continue // Retry
		}

		finalSize := startOffset + written
		if optMaxBytesFile > 0 && finalSize > optMaxBytesFile {
			os.Remove(partPath)
			return 0, nil, fmt.Errorf("download exceeded max-bytes-file (%d)", optMaxBytesFile)
		}
		if remainingTargetBudget > 0 && finalSize > remainingTargetBudget {
			os.Remove(partPath)
			return 0, nil, fmt.Errorf("download exceeded remaining max-bytes-target budget (%d)", remainingTargetBudget)
		}

		os.Remove(fullPath)
		if err := os.Rename(partPath, fullPath); err != nil {
			return 0, nil, err
		}

		sum := hex.EncodeToString(h.Sum(nil))
		finalURL := ""
		if resp != nil && resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		meta := &DownloadedMeta{
			FinalURL:    finalURL,
			ContentType: resp.Header.Get("Content-Type"),
			Size:        finalSize,
			SHA256:      sum,
		}

		return finalSize - startOffset, meta, nil
	}

	return 0, nil, fmt.Errorf("failed after %d attempts", maxRetries)
}

func preflightDownload(client *http.Client, u string) (contentType string, contentLength int64, finalURL string, ok bool) {
	req, err := http.NewRequest("HEAD", u, nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.String()
			}
			return resp.Header.Get("Content-Type"), resp.ContentLength, finalURL, true
		}
	}

	// Fallback: Range GET for 1 byte to harvest headers
	req, err = http.NewRequest("GET", u, nil)
	if err != nil {
		return "", 0, "", false
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, "", false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return resp.Header.Get("Content-Type"), resp.ContentLength, finalURL, true
}

func shouldRejectContentType(category, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	if strings.Contains(ct, "text/html") {
		switch category {
		case "documents", "archives", "audio", "images", "videos", "executables":
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
			resMu.Unlock()
			return
		}
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
		resMu.Unlock()

		filename := generateFilename(fullURL, category)
		info := FileInfo{
			URL:      fullURL,
			Category: category,
			Filename: filename,
		}

		resMu.Lock()
		if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
			resMu.Unlock()
			return
		}
		if capturedFiles[fullURL] {
			resMu.Unlock()
			continue
		}
		capturedFiles[fullURL] = true
		data.files = append(data.files, info)
		resMu.Unlock()

		fmt.Printf("[FOUND] [%s] %s\n", category, filepath.Base(filename))
	}
}

func captureFileURL(fileURL string, data *ScrapedData, capturedFiles map[string]bool, resMu *sync.Mutex) {
	lowerURL := strings.ToLower(fileURL)
	category := detectCategory(lowerURL)
	if category == "" {
		return
	}

	resMu.Lock()
	if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
		resMu.Unlock()
		return
	}
	if capturedFiles[fileURL] {
		resMu.Unlock()
		return
	}
	capturedFiles[fileURL] = true
	resMu.Unlock()

	filename := generateFilename(fileURL, category)
	info := FileInfo{URL: fileURL, Category: category, Filename: filename}

	resMu.Lock()
	if optMaxFiles > 0 && len(data.files) >= optMaxFiles {
		resMu.Unlock()
		return
	}
	data.files = append(data.files, info)
	resMu.Unlock()

	fmt.Printf("[FOUND] [%s] %s\n", category, filepath.Base(filename))
}

func parseCSVSet(s string) map[string]bool {
	out := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		out[p] = true
	}
	return out
}

func hasAnyQueryKey(rawURL string, deny map[string]bool) bool {
	if len(deny) == 0 {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	q := u.Query()
	for k := range q {
		if deny[strings.ToLower(k)] {
			return true
		}
	}
	return false
}

func canonicalHost(u *url.URL) string {
	host := strings.ToLower(u.Host)
	if host == "" {
		return ""
	}
	// If host contains port, normalize and remove default ports
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	if (u.Scheme == "http" && p == "80") || (u.Scheme == "https" && p == "443") {
		return h
	}
	return net.JoinHostPort(h, p)
}

func canonicalizeForVisit(rawURL string, deny map[string]bool) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		// Default to http for onion targets
		u.Scheme = "http"
	}
	u.Fragment = ""

	// Drop query for dedupe/crawl stability; denyQueryKeys is enforced separately.
	u.RawQuery = ""

	h := canonicalHost(u)
	if h == "" {
		return "", fmt.Errorf("missing host")
	}
	u.Host = h

	cleaned := path.Clean(u.Path)
	if cleaned == "." {
		cleaned = "/"
	}
	// Preserve trailing slash when it was explicit and not root
	if strings.HasSuffix(u.Path, "/") && cleaned != "/" {
		cleaned += "/"
	}
	u.Path = cleaned

	return u.String(), nil
}

func dumpPageFailure(onionAddr string, page playwright.Page, reason string) error {
	domainFolder := extractDomainForFolder(onionAddr)
	safeDomain := strings.ReplaceAll(domainFolder, ":", "_")
	safeDomain = strings.ReplaceAll(safeDomain, "/", "_")
	baseDir := filepath.Join(optOutputDir, "metadata", "failures", safeDomain)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}
	stamp := time.Now().Format("20060102_150405")
	base := filepath.Join(baseDir, stamp+"_"+reason)
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(base + ".png"), FullPage: playwright.Bool(true)})
	if html, err := page.Content(); err == nil {
		_ = os.WriteFile(base+".html", []byte(html), 0644)
	}
	return nil
}

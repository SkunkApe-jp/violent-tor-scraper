#!/usr/bin/env python3
"""
Onion Indexer Scraper - Paginated Link Extractor (Pro Version)

Features:
- Paginated crawling (follows 'Next' links)
- Real-time SQLite database saving (prevents data loss)
- Standard text file export
- Modern Firefox 140 headers
"""

import requests
import time
import re
import random
import argparse
import os
import sqlite3
from urllib.parse import urlparse, urljoin

def is_valid_onion_url(url):
    """Check if the URL is a valid onion URL."""
    try:
        parsed = urlparse(url)
        return parsed.netloc.endswith('.onion') if parsed.netloc else url.endswith('.onion')
    except:
        return False

def extract_onion_urls_from_text(text, base_url=None):
    """Extract .onion URLs from text, including relative links."""
    urls = []
    
    # Pattern 1: Full .onion URLs
    onion_pattern = r'(?:https?://)?([a-z0-9]+\.onion(?:[^\s"\'<>]*)?)'
    matches = re.findall(onion_pattern, text, re.IGNORECASE)
    for match in matches:
        url = match if match.startswith('http') else f'http://{match}'
        if is_valid_onion_url(url):
            parsed = urlparse(url)
            normalized = f"{parsed.scheme}://{parsed.netloc}{parsed.path}"
            if normalized.endswith('/'):
                normalized = normalized[:-1]
            urls.append(normalized)
    
    # Pattern 2: Relative links (href="/path" or href="path")
    if base_url:
        base_parsed = urlparse(base_url)
        base_domain = f"{base_parsed.scheme}://{base_parsed.netloc}"
        
        # href links that don't start with http
        rel_pattern = r'href=["\']((?!http|mailto|javascript)[^"\']+)["\']'
        rel_matches = re.findall(rel_pattern, text, re.IGNORECASE)
        for match in rel_matches:
            full_url = urljoin(base_url, match)
            if full_url.startswith(base_domain):
                parsed = urlparse(full_url)
                normalized = f"{parsed.scheme}://{parsed.netloc}{parsed.path}"
                if normalized.endswith('/'):
                    normalized = normalized[:-1]
                urls.append(normalized)
    
    return list(set(urls))

class OnionIndexerPro:
    def __init__(self, db_name="onion_database.db", timeout=30, delay=None, max_pages=100):
        self.timeout = timeout
        self.delay = delay
        self.max_pages = max_pages
        self.db_name = db_name
        self.proxies = {
            'http': 'socks5h://127.0.0.1:9050',
            'https': 'socks5h://127.0.0.1:9050'
        }
        self.headers = {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0',
            'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8',
            'Accept-Language': 'en-US,en;q=0.5',
            'Connection': 'keep-alive',
        }
        self.discovered_onions = set()
        self.visited_pages = set()
        self.setup_db()

    def setup_db(self):
        """Initialize the SQLite database."""
        conn = sqlite3.connect(self.db_name)
        cursor = conn.cursor()
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS sites (
                url TEXT PRIMARY KEY,
                discovery_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                source_page TEXT
            )
        ''')
        conn.commit()
        conn.close()

    def save_to_db(self, urls, source_page):
        """Save a list of URLs to the database in real-time."""
        conn = sqlite3.connect(self.db_name)
        cursor = conn.cursor()
        new_count = 0
        for url in urls:
            try:
                cursor.execute('INSERT INTO sites (url, source_page) VALUES (?, ?)', (url, source_page))
                new_count += 1
            except sqlite3.IntegrityError:
                continue # Already in DB
        conn.commit()
        conn.close()
        return new_count

    def find_next_page(self, html_content, current_url):
        """Detect the next page based on provided HTML structure."""
        current_page_match = re.search(r'<b>(\d+)</b>', html_content)
        if current_page_match:
            next_page_num = int(current_page_match.group(1)) + 1
            next_link_pattern = rf'href=["\']([^"\']*?page={next_page_num}[^"\']*)["\']'
            next_link_match = re.search(next_link_pattern, html_content)
            if next_link_match:
                return urljoin(current_url, next_link_match.group(1))

        # Generic fallback
        next_patterns = [
            r'<a[^>]+href=["\']([^"\']+)["\'][^>]*>[^<]*Next[^<]*</a>',
            r'<a[^>]+href=["\']([^"\']+)["\'][^>]*>[^<]*&gt;[^<]*</a>'
        ]
        for pattern in next_patterns:
            match = re.search(pattern, html_content, re.IGNORECASE)
            if match:
                return urljoin(current_url, match.group(1))
        return None

    def scrape(self, start_url):
        current_url = start_url
        page_count = 0
        
        print(f"[*] Starting 'Pro' scrape (Database: {self.db_name})")
        print(f"[*] Target URL: {current_url}")
        
        while current_url and page_count < self.max_pages:
            if current_url in self.visited_pages:
                break
            
            print(f"[*] Page {page_count + 1}: {current_url}")
            self.visited_pages.add(current_url)
            page_count += 1
            
            try:
                print(f"  [~] Making request...")
                response = requests.get(current_url, proxies=self.proxies, headers=self.headers, timeout=self.timeout)
                print(f"  [+] Response status: {response.status_code}")
                print(f"  [+] Response length: {len(response.text)} bytes")
                response.raise_for_status()
                
                print(f"  [~] Extracting onion URLs from response...")
                found_onions = extract_onion_urls_from_text(response.text, base_url=current_url)
                print(f"  [+] Raw .onion matches: {len(re.findall(r'[a-z0-9]+\.onion', response.text, re.IGNORECASE))}")
                print(f"  [+] Raw href links: {len(re.findall(r'href=[\"\']((?!http|mailto|javascript)[^\"\']+)[\"\']', response.text, re.IGNORECASE))}")
                print(f"  [+] Extracted URLs: {found_onions[:5]}{'...' if len(found_onions) > 5 else ''}")
                
                # REAL-TIME SAVE
                new_count = self.save_to_db(found_onions, current_url)
                for onion in found_onions:
                    self.discovered_onions.add(onion)
                
                print(f"  [+] Page results: {len(found_onions)} found, {new_count} new to database.")
                
                next_url = self.find_next_page(response.text, current_url)
                if next_url:
                    current_url = next_url
                    # Use specified delay or random delay between 1 and 5 seconds
                    wait_time = self.delay if self.delay is not None else random.uniform(1.0, 5.0)
                    if self.delay is None:
                        print(f"  [~] Sleeping for {wait_time:.2f}s (randomized)...")
                    else:
                        print(f"  [~] Sleeping for {wait_time:.2f}s...")
                    time.sleep(wait_time)
                else:
                    current_url = None
                    
            except Exception as e:
                print(f"  [-] Error: {e}")
                break
        
        print(f"[*] Finished. Total unique: {len(self.discovered_onions)}")

    def export_text(self, filename="indexed_onions.txt"):
        """Export everything from DB to a text file for other tools."""
        conn = sqlite3.connect(self.db_name)
        cursor = conn.cursor()
        cursor.execute('SELECT url FROM sites ORDER BY url')
        rows = cursor.fetchall()
        
        with open(filename, 'w', encoding='utf-8') as f:
            for row in rows:
                f.write(f"{row[0]}\n")
        conn.close()
        print(f"[✓] Exported {len(rows)} links to {filename}")

def main():
    parser = argparse.ArgumentParser(description='Onion Indexer Pro')
    parser.add_argument('--url', help='Starting URL')
    parser.add_argument('--file', help='File containing URLs (one per line)')
    parser.add_argument('--db', default='onion_database.db', help='SQLite database name')
    parser.add_argument('--output', default='indexed_onions.txt', help='Text export filename')
    parser.add_argument('--delay', type=float, default=None, help='Delay between requests (default: random 1-5s)')
    parser.add_argument('--pages', type=int, default=50)
    
    args = parser.parse_args()
    
    if not args.url and not args.file:
        parser.error('Either --url or --file must be specified')
    
    urls_to_process = []
    
    if args.url:
        urls_to_process.append(args.url)
    
    if args.file:
        if not os.path.exists(args.file):
            print(f"[-] Error: File not found: {args.file}")
            return
        with open(args.file, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#'):
                    urls_to_process.append(line)
        print(f"[*] Loaded {len(urls_to_process)} URL(s) from {args.file}")
    
    scraper = OnionIndexerPro(db_name=args.db, max_pages=args.pages)
    
    for i, url in enumerate(urls_to_process):
        print(f"\n[*] Processing URL {i+1}/{len(urls_to_process)}: {url}")
        scraper.scrape(url)
    
    scraper.export_text(args.output)

if __name__ == "__main__":
    main()

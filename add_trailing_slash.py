#!/usr/bin/env python3
"""
Add trailing slash to URLs in a file.
Processes each line and ensures URLs end with '/'.
"""

import argparse
import os

def add_trailing_slash(input_file, output_file=None, in_place=False):
    """Add trailing slash to each URL in the file."""
    if not os.path.exists(input_file):
        print(f"[-] Error: File not found: {input_file}")
        return
    
    if in_place:
        output_file = input_file
    elif not output_file:
        base, ext = os.path.splitext(input_file)
        output_file = f"{base}_with_slashes{ext}"
    
    modified_lines = []
    with open(input_file, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.rstrip('\n\r')
            if line and not line.endswith('/'):
                line += '/'
            modified_lines.append(line)
    
    with open(output_file, 'w', encoding='utf-8') as f:
        for line in modified_lines:
            f.write(f"{line}\n")
    
    print(f"[+] Processed {len(modified_lines)} lines")
    print(f"[+] Output: {output_file}")

def main():
    parser = argparse.ArgumentParser(description='Add trailing slash to URLs')
    parser.add_argument('input_file', help='Input file containing URLs')
    parser.add_argument('-o', '--output', help='Output file (default: input_with_slashes.txt)')
    parser.add_argument('-i', '--in-place', action='store_true', help='Modify input file in place')
    
    args = parser.parse_args()
    add_trailing_slash(args.input_file, args.output, args.in_place)

if __name__ == "__main__":
    main()

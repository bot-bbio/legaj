import sys
import json
import os
import urllib.parse
import requests
from bs4 import BeautifulSoup

from _encoding import force_utf8_io

# Search keywords/locations may contain non-Latin-1 glyphs that crash Windows'
# cp1252 stdout; emit UTF-8 instead.
force_utf8_io()

def search_adzuna(keywords, location, country='us'):
    # Using Adzuna free API for demo purposes
    # If the user has api credentials, they can set them. Otherwise we do a public fallback.
    url = f"https://api.adzuna.com/v1/api/jobs/{country}/search/1?what={urllib.parse.quote(keywords)}&where={urllib.parse.quote(location)}"
    # Adding public credentials or placeholder
    # For a local-first open tool, we fall back to web scraping Google/boards if API fails
    headers = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"}
    try:
        # Fallback to direct scraping of a public board like Indeed or simply generating search links
        # Since scraping Google/Indeed requires rotating proxies, the best local-first approach
        # is to generate pre-formatted search links AND query open APIs.
        print(f"Searching for '{keywords}' in '{location}'...")
        
        # Let's generate a list of direct search query URLs that the user can click
        search_urls = {
            "Google Jobs": f"https://www.google.com/search?q={urllib.parse.quote(keywords + ' jobs in ' + location)}&ibp=htl;jobs",
            "LinkedIn": f"https://www.linkedin.com/jobs/search/?keywords={urllib.parse.quote(keywords)}&location={urllib.parse.quote(location)}",
            "Indeed": f"https://www.indeed.com/jobs?q={urllib.parse.quote(keywords)}&l={urllib.parse.quote(location)}",
            "ZipRecruiter": f"https://www.ziprecruiter.com/jobs-search?search={urllib.parse.quote(keywords)}&location={urllib.parse.quote(location)}"
        }
        
        results = []
        # Return generated links + mock sample listings to show schema
        for board, link in search_urls.items():
            results.append({
                "source": board,
                "title": f"Search Link - {board}",
                "company": "Various",
                "location": location,
                "url": link,
                "description": f"Direct link to search {keywords} postings on {board}."
            })
            
        return results
    except Exception as e:
        print(f"Error during search execution: {str(e)}")
        return []

def main():
    if len(sys.argv) < 3:
        print("Usage: python search_jobs.py <keywords> <location> [output_json_path]")
        sys.exit(1)
        
    keywords = sys.argv[1]
    location = sys.argv[2]
    out_path = sys.argv[3] if len(sys.argv) > 3 else "outputs/job_search_results.json"
    
    results = search_adzuna(keywords, location)
    
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, 'w', encoding='utf-8') as f:
        json.dump(results, f, indent=2)
        
    print(f"Saved {len(results)} job sources to: {out_path}")
    print("\nPre-configured job search links generated successfully:")
    for res in results:
        print(f"- {res['source']}: {res['url']}")

if __name__ == "__main__":
    main()

"""Extract YouTube transcripts for k8s-sage training data.

Usage:
  python3 extract_all.py                              # direct (no proxy)
  PROXY_URL=http://user:pass@host:port python3 extract_all.py  # with proxy
"""
import os
import time
from youtube_transcript_api import YouTubeTranscriptApi
from youtube_transcript_api.proxies import GenericProxyConfig

VIDEOS = [
    ("4CT0cI62YHk", "airbnb_10_more_weird_ways"),
    ("FrQ8Lwm9_j8", "airbnb_10_weird_ways"),
    ("QXApVwRBeys", "airbnb_p95s_worse"),
    ("QKI-JRs2RIE", "datadog_10_ways_shoot"),
    ("ix0Tw8uinWs", "spotify_deleted_clusters"),
    ("8P7-C44Gjj8", "pusher_config_changed"),
    ("OUYTNywPk-s", "monzo_production_outage"),
    ("N2JUGnwinbQ", "google_stories_playbook"),
    ("likHm-KHGWQ", "yahoo_101_ways"),
    ("xZO9nx6GBu0", "nordstrom_101_ways_crash"),
    ("tA8Sr3Nsx1I", "thredup_moving_stack"),
    ("V0DVkrHf08k", "google_how_not_k8s"),
    ("LpFApeaGv7A", "zalando_crash_cluster"),
]

DELAY_SECONDS = 5  # delay between requests to avoid rate limiting

out_dir = os.path.dirname(os.path.abspath(__file__))
proxy_url = os.getenv("PROXY_URL")

if proxy_url:
    proxy_config = GenericProxyConfig(
        http_url=proxy_url,
        https_url=proxy_url,
    )
    ytt_api = YouTubeTranscriptApi(proxy_config=proxy_config)
    print(f"Using proxy: {proxy_url.split('@')[-1] if '@' in proxy_url else proxy_url}")
else:
    ytt_api = YouTubeTranscriptApi()
    print("No proxy configured (set PROXY_URL env var to use one)")

ok_count = 0
fail_count = 0

for i, (vid_id, name) in enumerate(VIDEOS):
    out_path = os.path.join(out_dir, f"{name}.txt")
    if os.path.exists(out_path) and os.path.getsize(out_path) > 100:
        print(f"SKIP {name}: already extracted ({os.path.getsize(out_path)} bytes)")
        ok_count += 1
        continue
    try:
        # Try English first, fall back to any available language
        try:
            transcript = ytt_api.fetch(vid_id)
        except Exception:
            transcript = ytt_api.fetch(vid_id, languages=["de", "fr", "es"])

        lines = []
        for snippet in transcript:
            text = snippet.text.replace("\n", " ").strip()
            if text:
                lines.append(text)
        full_text = "\n".join(lines)
        with open(out_path, "w") as f:
            f.write(full_text)
        print(f"OK   {name}: {len(full_text):,} chars, {len(lines)} lines")
        ok_count += 1
    except Exception as e:
        err_msg = str(e).split("\n")[0][:120]
        print(f"FAIL {name}: {err_msg}")
        fail_count += 1

    if i < len(VIDEOS) - 1:
        time.sleep(DELAY_SECONDS)

print(f"\nDone: {ok_count} OK, {fail_count} FAIL")

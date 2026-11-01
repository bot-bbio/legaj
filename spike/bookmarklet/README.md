# Bookmarklet handoff spike (throwaway)

This tests whether a bookmarklet can hand a job page to a popup served by LeGaJ on `127.0.0.1`, using `postMessage` or a URL fragment, on real job sites in Edge and Chrome.

```
go run ./spike/bookmarklet          # then open http://127.0.0.1:47321/
```

- Each attempt is logged to `spike/bookmarklet/out/results.jsonl` (sizes and flags only).
- **Save capture** stores the full payload in `out/captures/` as a fixture candidate. Captures can contain personal data. The `out/` folder is gitignored.

This folder is deleted before the V2 branch is merged.

#!/usr/bin/env bash
set -euo pipefail

# Regenerate the precompiled Tailwind CSS for the GitHub Pages landing.
#
# The docs/ directory is served as static files by GitHub Pages, so the CSS
# must be precompiled and committed. Run this script after editing index.html
# or app.js (they are scanned via the `content` globs in tailwind.config.js).
#
# Requires Node.js. Uses Tailwind v3 to match the previous CDN behaviour.

cd "$(dirname "$0")"

npx tailwindcss@3 -c tailwind.config.js -i tailwind.input.css -o tailwind.css --minify

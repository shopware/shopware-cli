#!/bin/bash
# Read-only evidence for the shopware-cli-extension-store skill.
# Usage: collect-evidence.sh [extension-root]   (default: current directory)
# Exit 1 with EVIDENCE_INCOMPLETE lines when a step fails.
set -u

cd "${1:-.}" || exit 1

CLI="$(command -v shopware-cli || true)"
if [ -z "$CLI" ]; then
  echo "FATAL: no shopware-cli available"; exit 1
fi

status=0
fail() { echo "EVIDENCE_INCOMPLETE: $*"; status=1; }

echo '--- provenance ---'
printf 'cli_path: %s\n' "$CLI"
"$CLI" --version || fail "shopware-cli --version failed"
date -u +'inspected_at: %Y-%m-%dT%H:%M:%SZ'

echo '--- files ---'
find . -maxdepth 5 -type f \
  -not -path './vendor/*' -not -path './node_modules/*' -not -path './.git/*' | sort \
  || fail "find failed"

echo '--- kind ---'
# Apps have Resources/ in the root, plugins in src/.
if [ -f manifest.xml ]; then
  echo 'kind: app'
  resources=Resources
elif grep -q '"type": *"shopware-platform-plugin"' composer.json 2>/dev/null; then
  echo 'kind: plugin'
  resources=src/Resources
else
  echo 'kind: unknown (no manifest.xml; composer.json is missing or not a shopware-platform-plugin)'
  resources=src/Resources
fi
# Theme: src/Resources/theme.json or ThemeInterface in src/; vendor/ is not searched.
if [ -f "$resources/theme.json" ] || grep -rq 'ThemeInterface' src --include='*.php' 2>/dev/null; then
  echo 'kind: theme'
else
  echo 'kind: not a theme'
fi

echo '--- metadata ---'
# Keep every description locale, the authors key itself, and the label and link maps.
# Lengths are Unicode code points, as in the CLI.
if [ ! -f composer.json ]; then
  echo 'composer.json: absent (apps carry their metadata in manifest.xml)'
elif command -v jq >/dev/null; then
  # jq is not always installed; php is, python3 usually.
  jq '{license, label:.extra.label, authors_key_present:has("authors"), authors,
       description_lengths:((.extra.description // {}) | map_values(length)),
       manufacturerLink:.extra.manufacturerLink, supportLink:.extra.supportLink}' composer.json \
    || fail "jq could not read composer.json"
elif command -v php >/dev/null; then
  php -r '$j=json_decode(file_get_contents("composer.json"),true);
    if(!is_array($j)){fwrite(STDERR,"invalid composer.json\n");exit(1);}
    $e=$j["extra"]??[];$d=[];foreach(($e["description"]??[]) as $k=>$v){$d[$k]=preg_match_all("/./us",(string)$v);}
    echo json_encode([
      "license"=>$j["license"]??null,
      "label"=>$e["label"]??null,
      "authors_key_present"=>array_key_exists("authors",$j),
      "authors"=>$j["authors"]??null,
      "description_lengths"=>(object)$d,
      "manufacturerLink"=>$e["manufacturerLink"]??null,
      "supportLink"=>$e["supportLink"]??null,
    ],JSON_PRETTY_PRINT|JSON_UNESCAPED_SLASHES),"\n";' \
    || fail "php could not read composer.json"
elif command -v python3 >/dev/null; then
  python3 -c 'import json;j=json.load(open("composer.json"));e=j.get("extra",{});print(json.dumps({"license":j.get("license"),"label":e.get("label"),"authors_key_present":"authors" in j,"authors":j.get("authors"),"description_lengths":{k:len(v) for k,v in (e.get("description") or {}).items()},"manufacturerLink":e.get("manufacturerLink"),"supportLink":e.get("supportLink")},indent=2))' \
    || fail "python3 could not read composer.json"
else
  fail "no jq, php or python3 available to read composer.json"
fi
[ -f .shopware-extension.yml ] && cat .shopware-extension.yml

echo '--- icon ---'
icon="$resources/config/plugin.png"
if [ -f "$icon" ]; then
  file "$icon" 2>/dev/null || echo "$icon: file(1) unavailable, dimensions not measured"
  printf 'bytes: %s\n' "$(wc -c < "$icon" | tr -d ' ')"
else
  echo "plugin icon absent ($icon)"
fi

echo '--- preconditions ---'
ls "$resources/config/config.xml" 2>/dev/null || echo 'no config.xml'
# User-facing text: snippet files, error or notification code.
find "$resources" -path '*snippet*' -name '*.json' 2>/dev/null | head -5
grep -rlE '\.error\(|createNotification|notification' "$resources/app" 2>/dev/null | head -5
# CMS elements: registerCmsElement (admin) or cms-element templates (storefront).
grep -rlE 'registerCmsElement|cms-element' "$resources" 2>/dev/null | head -5 || true
grep -rqE 'registerCmsElement|cms-element' "$resources" 2>/dev/null || echo 'no CMS element found'

# The report goes to stdout, the usage block and error line go to stderr; both are kept.
raw="$(mktemp -d "${TMPDIR:-/tmp}/sw-store-evidence.XXXXXX")" || { fail "cannot create the raw output directory"; raw=/tmp; }
findings() { grep -E '^- \*\*' "$1" 2>/dev/null || true; }

echo '--- validation: normal ---'
"$CLI" extension validate . --format markdown >"$raw/normal.md" 2>"$raw/normal.log"; rc=$?
findings "$raw/normal.md" | LC_ALL=C sort -u > "$raw/normal.lines"
# Print each finding once; the count of collapsed repeats is kept visible.
findings "$raw/normal.md" | awk '!seen[$0]++'
dups=$(( $(findings "$raw/normal.md" | wc -l) - $(wc -l < "$raw/normal.lines") ))
[ "$dups" -gt 0 ] && echo "duplicate lines collapsed: $dups"
grep -m1 'No problems found' "$raw/normal.md" || true
[ ! -s "$raw/normal.lines" ] && ! grep -q 'No problems found' "$raw/normal.md" && { echo 'no report on stdout, stderr follows:'; head -3 "$raw/normal.log"; }
echo "normal_exit=$rc"

# Only the difference to the normal run is printed; identical output is not repeated.
echo '--- validation: store-compliance ---'
"$CLI" extension validate . --store-compliance --format markdown >"$raw/store.md" 2>"$raw/store.log"; rc=$?
findings "$raw/store.md" | LC_ALL=C sort -u > "$raw/store.lines"
added="$(LC_ALL=C comm -13 "$raw/normal.lines" "$raw/store.lines")"
removed="$(LC_ALL=C comm -23 "$raw/normal.lines" "$raw/store.lines")"
if [ -z "$added" ] && [ -z "$removed" ]; then
  echo 'findings: identical to the normal run'
else
  [ -n "$added" ] && { echo 'only in the store-compliance run:'; printf '%s\n' "$added"; }
  [ -n "$removed" ] && { echo 'only in the normal run:'; printf '%s\n' "$removed"; }
fi
echo "store_exit=$rc"

echo '--- raw ---'
ls "$raw"/normal.md "$raw"/normal.log "$raw"/store.md "$raw"/store.log 2>/dev/null

[ "$status" -ne 0 ] && echo 'EVIDENCE_INCOMPLETE: one or more collection steps failed, see lines above'
exit "$status"

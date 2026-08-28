#!/bin/bash
set -u

cd "${1:-.}" || exit 1

CLI="$(command -v shopware-cli || true)"
if [ -x ../shopware-cli/bin/shopware-cli ]; then
  CLI="$(cd ../shopware-cli/bin && pwd)/shopware-cli"
fi
if [ -z "$CLI" ]; then
  echo "FATAL: no shopware-cli available"; exit 1
fi

echo '--- provenance ---'
printf 'cli_path: %s\n' "$CLI"
"$CLI" --version
date -u +'inspected_at: %Y-%m-%dT%H:%M:%SZ'
# if a shopware-cli source checkout is present, pin the exact revision the rules were read from
if [ -d ../shopware-cli/.git ]; then
  printf 'cli_source_rev: %s\n' "$(git -C ../shopware-cli rev-parse --short HEAD)"
fi

echo '--- files ---'
find . -maxdepth 5 -type f \
  -not -path './vendor/*' -not -path './node_modules/*' -not -path './.git/*' | sort

echo '--- metadata ---'
# jq is not guaranteed on a Shopware dev box; php always is.
if command -v jq >/dev/null; then
  jq -r '{license, label:.extra.label, authors:(.authors|length),
          desc_en:(.extra.description."en-GB"//""|length),
          desc_de:(.extra.description."de-DE"//""|length),
          manufacturerLink:.extra.manufacturerLink,
          supportLink:.extra.supportLink}' composer.json
elif command -v php >/dev/null; then
  php -r '$j=json_decode(file_get_contents("composer.json"),true);$e=$j["extra"]??[];
    echo json_encode([
      "license"=>$j["license"]??null,
      "label"=>$e["label"]??null,
      "authors"=>count($j["authors"]??[]),
      "desc_en"=>mb_strlen($e["description"]["en-GB"]??"","UTF-8"),
      "desc_de"=>mb_strlen($e["description"]["de-DE"]??"","UTF-8"),
      "manufacturerLink"=>$e["manufacturerLink"]??null,
      "supportLink"=>$e["supportLink"]??null,
    ],JSON_PRETTY_PRINT|JSON_UNESCAPED_SLASHES),"\n";'
else
  python3 -c 'import json;j=json.load(open("composer.json"));e=j.get("extra",{});print(json.dumps({"license":j.get("license"),"label":e.get("label"),"authors":len(j.get("authors",[])),"desc_en":len(e.get("description",{}).get("en-GB","")),"desc_de":len(e.get("description",{}).get("de-DE","")),"manufacturerLink":e.get("manufacturerLink"),"supportLink":e.get("supportLink")},indent=2))'
fi
[ -f .shopware-extension.yml ] && cat .shopware-extension.yml

echo '--- icon ---'
if [ -f src/Resources/config/plugin.png ]; then
  file src/Resources/config/plugin.png
  wc -c < src/Resources/config/plugin.png
else
  echo 'plugin icon absent'
fi

echo '--- preconditions ---'
ls src/Resources/config/config.xml 2>/dev/null || echo 'no config.xml'
grep -rl 'snippet\|error' src/Resources/app 2>/dev/null | head -5 || true
grep -q '"type": *"shopware-platform-plugin"' composer.json && echo 'kind: plugin'
# quote the --include globs: zsh errors on unmatched globs and aborts the block
grep -rq 'ThemeInterface\|theme.json' . '--include=*.php' '--include=*.json' 2>/dev/null \
  && echo 'kind: theme' || echo 'kind: not a theme'

# capture into a variable, not a pipe: PIPESTATUS is bash-only and $? after a
# pipe reports sed, not the CLI
echo '--- validation: normal ---'
out="$("$CLI" extension validate . --reporter markdown 2>&1)"; rc=$?
printf '%s\n' "$out" | sed '/^Usage:/,$d'
echo "normal_exit=$rc"

echo '--- validation: store-compliance ---'
out="$("$CLI" extension validate . --store-compliance --reporter markdown 2>&1)"; rc=$?
printf '%s\n' "$out" | sed '/^Usage:/,$d'
echo "store_exit=$rc"

#!/bin/sh
# Refreshes the third-party assets embedded in vanitypages from npm.
# Run it after bumping a version below; the outputs are committed.
set -eu

UIKIT=8.0.0
ICONS=2.23.1
INTER=5.3.0

cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fetch() {
	mkdir "$tmp/$1"
	(cd "$tmp/$1" && npm pack --silent --ignore-scripts "$2" >/dev/null && tar xzf ./*.tgz)
}
fetch uikit "@gravity-ui/uikit@$UIKIT"
fetch icons "@gravity-ui/icons@$ICONS"
fetch inter "@fontsource-variable/inter@$INTER"

components="$tmp/uikit/package/build/esm/components"

# Design tokens (minus the high-contrast themes) plus the components the
# templates use, as one stylesheet.
{
	printf '/* Gravity UI uikit %s (MIT), see THIRD_PARTY_LICENSES.txt */\n' "$UIKIT"
	awk '/^\.g-root_theme_(light|dark)-hc \{$/ {skip=1} !skip {print} skip && /^\}$/ {skip=0}' \
		"$tmp/uikit/package/styles/styles.css"
	for css in Button/Button Card/Card Icon/Icon Label/Label Link/Link Text/text/text Text/colorText/colorText; do
		cat "$components/$css.css"
		echo
	done
} | grep -v -e '^@charset' -e 'sourceMappingURL' >gravity.gen.css

for icon in arrow-up-right-from-square book-open check chevron-right code copy cube moon sun; do
	printf '{{define "icon-%s"}}' "$icon"
	sed 's/<svg /<svg class="g-icon" aria-hidden="true" /' "$tmp/icons/package/svgs/$icon.svg" | tr -d '\n'
	printf '{{end}}\n'
done >templates/icons.gen.html

cp "$tmp/inter/package/files/inter-latin-wght-normal.woff2" inter-latin.woff2
cp "$tmp/inter/package/files/inter-cyrillic-wght-normal.woff2" inter-cyrillic.woff2

{
	printf '@gravity-ui/uikit %s\n\n' "$UIKIT"
	cat "$tmp/uikit/package/LICENSE"
	printf '\n\n@gravity-ui/icons %s\n\n' "$ICONS"
	cat "$tmp/icons/package/LICENSE"
	printf '\n\nInter (@fontsource-variable/inter %s)\n\n' "$INTER"
	cat "$tmp/inter/package/LICENSE"
} >THIRD_PARTY_LICENSES.txt

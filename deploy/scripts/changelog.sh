#!/bin/bash
# Generate RPM changelogs, AppStream releases, and release notes from CHANGELOG.md.
#
#   latest                 newest x.y.z heading
#   check VERSION          verify version, release dates, and empty [Unreleased]
#   notes VERSION          Markdown body of VERSION
#   rpm [VERSION RELEASE]  RPM %changelog with RELEASE for the specified VERSION
#   appstream              AppStream <releases> block
#   roll VERSION [DATE]    version and date the release entry
#
# Headings are "## [x.y.z] - YYYY-MM-DD". Non-semver labels are skipped by
# rpm and appstream.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHANGELOG="${CHANGELOG:-$(cd "${SCRIPT_DIR}/../.." && pwd)/CHANGELOG.md}"
RPM_AUTHOR="${RPM_CHANGELOG_AUTHOR:-Defenestra Team <support@defenestra.io>}"

fail() {
    echo "changelog.sh: $*" >&2
    exit 1
}

normalize() {
    local v="${1#v}"
    [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "'$1' is not a version of the form x.y.z"
    printf '%s' "$v"
}

read -r -d '' AWK_LIB <<'AWK' || true
function heading(line,   s) {
    if (line !~ /^## \[[^]]+\]/) return ""
    s = line
    sub(/^## \[/, "", s)
    sub(/\].*$/, "", s)
    return s
}
function heading_date(line) {
    if (match(line, /[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/))
        return substr(line, RSTART, RLENGTH)
    return ""
}
function valid_date(iso,   y, m, d, days) {
    if (iso !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) return 0
    y = substr(iso, 1, 4) + 0
    m = substr(iso, 6, 2) + 0
    d = substr(iso, 9, 2) + 0
    if (y < 1 || m < 1 || m > 12 || d < 1) return 0
    split("31 28 31 30 31 30 31 31 30 31 30 31", days, " ")
    if (y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)) days[2] = 29
    return d <= days[m]
}
function is_version(label) {
    return label ~ /^[0-9]+\.[0-9]+\.[0-9]+$/
}
AWK

validate_dates() {
    awk "$AWK_LIB"'
    { l = heading($0) }
    is_version(l) && !valid_date(heading_date($0)) {
        print "changelog.sh: release " l " needs a valid YYYY-MM-DD date" > "/dev/stderr"
        invalid = 1
    }
    END { exit invalid }
    ' "$CHANGELOG"
}

cmd_latest() {
    awk "$AWK_LIB"'
    { l = heading($0) }
    l != "" && is_version(l) { print l; exit }
    ' "$CHANGELOG"
}

cmd_check() {
    local want
    want=$(normalize "$1")
    validate_dates
    awk -v want="$want" "$AWK_LIB"'
    { l = heading($0) }
    l != "" {
        in_unreleased = (n_heading == 0 && l == "Unreleased")
        n_heading++
        if (latest == "" && is_version(l)) latest = l
        next
    }
    in_unreleased && $0 !~ /^[[:space:]]*$/ { pending++ }
    END {
        status = 0
        if (pending) {
            print "changelog.sh: [Unreleased] still holds " pending " line(s); roll them with: task tag VERSION=" want > "/dev/stderr"
            status = 1
        }
        if (latest == "") {
            print "changelog.sh: no released section in CHANGELOG.md" > "/dev/stderr"
            status = 1
        } else if (latest != want) {
            print "changelog.sh: newest section is " latest " but the tag is " want > "/dev/stderr"
            status = 1
        }
        exit status
    }' "$CHANGELOG"
}

cmd_notes() {
    local want
    want=$(normalize "$1")
    awk -v want="$want" "$AWK_LIB"'
    { l = heading($0) }
    l != "" {
        if (on) exit
        on = (l == want)
        if (on) found = 1
        next
    }
    on { body[n++] = $0 }
    END {
        if (!found) {
            print "changelog.sh: no section for " want " in CHANGELOG.md" > "/dev/stderr"
            exit 1
        }
        first = 0
        last = n - 1
        while (first <= last && body[first] ~ /^[[:space:]]*$/) first++
        while (last >= first && body[last] ~ /^[[:space:]]*$/) last--
        for (i = first; i <= last; i++) print body[i]
    }' "$CHANGELOG"
}

cmd_rpm() {
    local want="" rel="1"
    [ $# -ge 1 ] && want=$(normalize "$1")
    [ $# -ge 2 ] && rel="$2"
    validate_dates
    awk -v author="$RPM_AUTHOR" -v want="$want" -v rel="$rel" "$AWK_LIB"'
    function dow(y, m, d,   t) {
        # Sakamoto: 0 = Sunday.
        split("0 3 2 5 0 3 5 1 4 6 2 4", t, " ")
        if (m < 3) y--
        return (y + int(y / 4) - int(y / 100) + int(y / 400) + t[m] + d) % 7
    }
    function rpm_date(iso,   p, days, months) {
        split(iso, p, "-")
        split("Sun Mon Tue Wed Thu Fri Sat", days, " ")
        split("Jan Feb Mar Apr May Jun Jul Aug Sep Oct Nov Dec", months, " ")
        return days[dow(p[1] + 0, p[2] + 0, p[3] + 0) + 1] " " months[p[2] + 0] " " p[3] " " p[1]
    }
    { l = heading($0) }
    l != "" {
        on = is_version(l)
        if (!on) next
        if (entries++) print ""
        print "* " rpm_date(heading_date($0)) " " author " - " l "-" (l == want ? rel : 1)
        next
    }
    !on { next }
    /^### / || /^[[:space:]]*$/ { next }
    { gsub(/%/, "%%"); print }
    ' "$CHANGELOG"
}

cmd_appstream() {
    validate_dates
    awk "$AWK_LIB"'
    function xml(s) {
        gsub(/&/, "\\&amp;", s)
        gsub(/</, "\\&lt;", s)
        gsub(/>/, "\\&gt;", s)
        while (match(s, /`[^`]+`/))
            s = substr(s, 1, RSTART - 1) "<code>" substr(s, RSTART + 1, RLENGTH - 2) "</code>" substr(s, RSTART + RLENGTH)
        return s
    }
    function flush_item() {
        if (item != "") print "          <li>" xml(item) "</li>"
        item = ""
    }
    function close_list() {
        flush_item()
        if (in_list) print "        </ul>"
        in_list = 0
    }
    function close_release() {
        if (!on) return
        close_list()
        print "      </description>"
        print "    </release>"
    }
    BEGIN { print "  <releases>" }
    { l = heading($0) }
    l != "" {
        close_release()
        on = is_version(l)
        if (!on) next
        print "    <release version=\"" l "\" date=\"" heading_date($0) "\">"
        print "      <description>"
        next
    }
    !on { next }
    /^### / {
        close_list()
        t = $0
        sub(/^### +/, "", t)
        print "        <p>" xml(t) "</p>"
        next
    }
    /^- / {
        flush_item()
        if (!in_list) print "        <ul>"
        in_list = 1
        item = substr($0, 3)
        next
    }
    /^  +[^ ]/ && item != "" {
        t = $0
        sub(/^ +/, "", t)
        item = item " " t
        next
    }
    /^[[:space:]]*$/ { next }
    { close_list(); print "        <p>" xml($0) "</p>" }
    END { close_release(); print "  </releases>" }
    ' "$CHANGELOG"
}

cmd_roll() {
    local want date state has body latest tmp
    want=$(normalize "$1")
    date="${2:-$(date +%F)}"
    awk -v d="$date" "$AWK_LIB"'BEGIN { exit !valid_date(d) }' ||
        fail "'$date' is not a valid YYYY-MM-DD date"
    state=$(awk "$AWK_LIB"'
    { l = heading($0) }
    l != "" {
        n++
        if (n == 1 && l == "Unreleased") unreleased = 1
        else if (unreleased == 1) unreleased = 2
        if (latest == "" && is_version(l)) latest = l
        next
    }
    unreleased == 1 && $0 !~ /^[[:space:]]*$/ { content = 1 }
    END {
        print (unreleased ? "unreleased" : "none"), (content ? "content" : "empty"), (latest == "" ? "-" : latest)
    }' "$CHANGELOG")
    read -r has body latest <<<"$state"

    tmp=$(mktemp)
    trap 'rm -f "$tmp"' RETURN
    case "$has/$body" in
        unreleased/content)
            [ "$latest" != "$want" ] || fail "$want is already released but [Unreleased] holds new entries; pick a newer version"
            awk -v v="$want" -v d="$date" '
            !done && /^## \[Unreleased\]/ { print "## [Unreleased]"; print ""; print "## [" v "] - " d; done = 1; next }
            { print }' "$CHANGELOG" >"$tmp"
            ;;
        unreleased/empty)
            [ "$latest" = "$want" ] || fail "nothing to release: [Unreleased] is empty and the newest section is $latest"
            cat "$CHANGELOG" >"$tmp"
            ;;
        none/*)
            [ "$latest" = "$want" ] || fail "no [Unreleased] section and the newest section is $latest, not $want"
            awk '
            !done && /^## \[/ { print "## [Unreleased]"; print ""; done = 1 }
            { print }' "$CHANGELOG" >"$tmp"
            ;;
    esac
    # A prewritten release section may still need its release date.
    state=$(awk -v v="$want" -v d="$date" "$AWK_LIB"'
    heading($0) == v && heading_date($0) == "" { $0 = "## [" v "] - " d }
    { print }' "$tmp")
    printf '%s\n' "$state" >"$tmp"
    CHANGELOG="$tmp" validate_dates
    cat "$tmp" >"$CHANGELOG"
    echo "changelog.sh: rolled [Unreleased] into [$want] - $date"
}

[ -f "$CHANGELOG" ] || fail "$CHANGELOG not found"
cmd="${1:-}"
shift || true
case "$cmd" in
    latest) cmd_latest ;;
    check) [ $# -eq 1 ] || fail "usage: check VERSION"; cmd_check "$1" ;;
    notes) [ $# -eq 1 ] || fail "usage: notes VERSION"; cmd_notes "$1" ;;
    rpm) [ $# -eq 0 ] || [ $# -eq 2 ] || fail "usage: rpm [VERSION RELEASE]"; cmd_rpm "$@" ;;
    appstream) cmd_appstream ;;
    roll) [ $# -ge 1 ] && [ $# -le 2 ] || fail "usage: roll VERSION [DATE]"; cmd_roll "$@" ;;
    *) sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2 ;;
esac

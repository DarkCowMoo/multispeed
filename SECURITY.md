# Security

Please report security problems privately, not in a public issue: use
[Report a vulnerability](https://github.com/DarkCowMoo/multispeed/security/advisories/new)
on this repository's Security tab.

Only the latest release is supported. multispeed runs locally, talks to public Speedtest.net
servers, and (with `-route`) runs the system's `tracert`/`traceroute`; it has no server
component and stores only its own history file in your user config folder.

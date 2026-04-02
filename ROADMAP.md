# ROADMAP

## v2.x — Planned

### OS Keychain Integration

PATs are currently passed via environment variables. A future release will add optional keychain storage via [go-keyring](https://github.com/zalando/go-keyring), allowing users to store and retrieve tokens securely from the OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service).

```bash
# future API (not yet implemented)
deadgit org add mycompany --provider github --pat-env GITHUB_PAT --keychain
```

This is tracked as a ROADMAP item because go-keyring requires no external setup on macOS/Windows, making it a clean addition for Homebrew users.

### Additional Providers

- GitLab (cloud + self-hosted)
- Bitbucket Cloud

### Web Dashboard

Optional local web server for browsing scan results interactively.

## v3.x — Ideas

- Notifications (Slack/Teams webhook) when repos cross the inactive threshold
- Scheduled scans via system cron integration
- Export to Jira/GitHub Issues for remediation tracking

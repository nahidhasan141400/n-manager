# N Manager

N Manager runs on your computer. Each VPS is an SSH login. The sidebar dropdown chooses which server the Dashboard, Apps, Nginx, Firewall, and FTP pages talk to. The panel is not installed on those servers, and it does not open a shell from the browser.

The panel listens on `127.0.0.1:8080`.

## What you need

On your computer:

- Go and Node.js, to build the binary
- The panel process, started with `./n-manager --config dev-data/config.yaml` or your own config

On each VPS:

- Ubuntu, Debian, or a Fedora/RHEL-family system
- SSH on port 22, or another port you enter in the form
- An account that can install packages and control services
- `root` runs commands directly. Any other user is asked for the same password through `sudo -S`

## 1. Build and start the panel

```bash
git clone git@github.com:nahidhasan141400/n-manager.git
cd n-manager
./build.sh
./n-manager --config dev-data/config.yaml
```

Open http://127.0.0.1:8080 and sign in.

`security.dry_run` must be `false` when you want Install, Save, and service changes to reach the VPS. While it is `true`, those actions are checked and then refused with “dry run: no system changes were made”. Status pages still connect over SSH.

## 2. Add a server

Settings → Servers. The form is the SSH login:

- Name, so the dropdown can tell machines apart
- IP or hostname
- User
- Password
- Port, left at 22 unless you change it

Use **Test**, then **Use**. The same server stays selected after a refresh. Removing a server asks for confirmation.

The password is stored only in the local SQLite database, encrypted with a key from `security.session_secret`. The API does not return it, and the audit log does not record it.

The first successful connection saves the server's host key. A later key change is refused until you confirm the new fingerprint with **Trust**.

## 3. Manage that server

| Task | Where |
| --- | --- |
| CPU, memory, disk, network | Dashboard |
| MySQL databases, users, privileges, import and export | Apps |
| PostgreSQL service | Apps |
| Node.js versions and global packages | Apps |
| PM2 processes | Apps |
| Git install, GitHub SSH key, clone, and pull | Apps |
| Sites, certificates, Certbot | Nginx |
| UFW policies, logging, IPv6, and rules | Firewall |
| vsftpd users and transfers | FTP |
| SSH servers, password, session time | Settings |

With no server selected, those pages ask you to add one. They do not manage the computer running the panel.

Stop, disable, and delete ask for confirmation. MySQL stays on `127.0.0.1` of the selected VPS.

## Files on your computer

| Path | Purpose |
| --- | --- |
| `dev-data/config.yaml` | Bind address, password hash, session secret, dry run |
| `dev-data/nmanager.db` | Sessions, audit log, and encrypted server logins |

Change the admin password from Settings, or replace `security.admin_password_hash` with:

```bash
printf '%s' 'new-password' | ./n-manager hash-password
```

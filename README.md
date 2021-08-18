# sshwebdav: WebDAV server for SSH

`sshwebdav` provides a WebDAV server for a remote SSH host.

`sshwebdav` is similar to `sshfs` but does not require proprietary MacFUSE on macOS.

`sshwebdav` is planned to be integrated into [Lima](https://github.com/lima-vm/lima), for exposing
Linux (guest) filesystem to macOS (host).

## Install

```
make
make install
```

## Usage

```
sshwebdav ssh://foo@example.com:22/home/foo http://127.0.0.1:8080/
```

Open `Go` menu of macOS Finder, choose `Connect to Server`, and connect to http://127.0.0.1:8080 .

Current limitations:
- No support for write operations
- No support for HTTP auth
- No support for HTTPS, so make sure to serve only for 127.0.0.1

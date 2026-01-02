# Beluga

Sync video playback on Linux desktops with `mpv`.

## Server

Deploy it on a server which acts as a broker.

```sh
go install github.com/lavafroth/beluga/cmd/server@latest
nohup server &
disown -a
```

## Client

```sh
go install github.com/lavafroth/beluga/cmd/client@latest
cd directory-with-movie
client server-hostname-or-ip:8000
```

Bento is a service manager for OS X with a robust cli interface, and a light system tray UI. It’s designed for personal use, as opposed to init.d or upstart.

## Install Bento

The easiest is to use [Homebrew](http://brew.sh/): `brew install heewa/tap/bento` or `brew install --devel heewa/tap/bento` for the development version, updated more frequently (devel requires Go 1.5+).

## What Does Bento Do?

* Bento runs things in the background, so you don't have to leave terminals open for each of them.
```bash
$ bento run-once mongod
  ⌁ mongod               started now pid:13077 cmd:'mongod'
  
$ bento run-once redis-server
  ⌁ redis-server         started now pid:41059 cmd:'redis-server'

$ bento list
  ⌁ redis-server         started 2 seconds ago pid:41059 cmd:'redis-server'
  ⌁ mongod               started 4 seconds ago pid:13077 cmd:'mongod'
```

* Easily save services to start & stop quickly with simple yaml files.
```yaml
- name: Mongo
  program: mongod
  args: ["--config", "/path/to/mongo.conf"]
```
```bash
$ bento reload
Added 1 new services:
  ● Mongo                unstarted cmd:'mongod --config /path/to/mongo.conf'

$ bento start Mongo
  ⌁ Mongo             ↺  started now pid:15293 cmd:'mongod --config /path/to/mongo.conf'

$ bento stop Mongo
  ✔ Mongo             ↺  ended now pid:15293 cmd:'mongod --config /path/to/mongo.conf'
```

* Bento can keep services running by restarting them when they exit, and auto start on login.
```yaml
- name: WorkDashboardTunnel
  program: ssh
  args: ["-L", "8080:firewalled-server:80", "-N", "-n", "workuser@gateway.work.com"]
  auto-start: true
  restart-on-exit: true
```
```bash
$ bento reload
Added 1 new services:
  ⌁ WorkDashboardTunnel   ↺  started now pid:34303 cmd:'ssh -L 8080:firewalled-serve...'

$ sleep 10 ; kill $(bento pid WorkDashboardTunnel)

$ bento list
  ⌁ WorkDashboardTunnel   ↺  started now pid:34303 cmd:'ssh -L 8080:firewalled-serve...'
```

* Check on services whenever you want.
```bash
$ bento tail -n 100 redis # like regular tail command

$ bento tail -f redis # follows output from a running service, similar to tail -f

$ bento tail -F redis # follows restarts to a service, similar to tail -F
```

* Bento has bash tab completion.
```bash
$ bento start Wor<tab>

$ bento start WorkDashboardTunnel
```


## Saving services

You can save services permanently in a simple yaml file at `~/.bento/services.yml`. The file should contain a list of service definitions like:

```yaml
- name: Redis
  program: redis-server
  restart-on-exit: true
- name: Memcache
  program: memcached
  args: ['-v']
  auto-start: true
  restart-on-exit: true
```

After changing the file, reload the service configuration without restarting with: `bento reload`. If you're having trouble getting a service right, try running it as a temp service (`bento run-once --args cmd -- cmd-args`), then get a yaml config for it with `bento list -l` (long list).

### Service Configuration Options

* `name`: (required) The name of the service. You'll specify this to manage the service on the cli.
* `program`: (required) A full path to the binary to run. This is a regular path, not a bash command.
* `args`: A list of arguments to the program. Again, this isn't bash, so wildcards, `~`, and env vars don't work. If you really want these, let me know in a github issue or email, and I'll try to get that feature in sooner.
* `dir`: A path to a runtime dir for the program. It defaults to the home dir of the server's starting user.
* `env`: A map of environment variable names to values.
* `auto-start`: If true, this service will be automatically started by bento when it first runs.
* `restart-on-exit`: If true, bento will attempt to restart the service if it exits. However, `bento stop` will cause the service to stop until explicitly started again.

## Building

To build it, you need to have a Go environment set up, then `go get -v github.com/heewa/bento`, update with `go get -u -v github.com/heewa/bento`. If just running `bento` doesn’t work after that, you might need to set add `$GOPATH/bin` to your `$PATH` env var.

If you also installed bento with Homebrew, you'll already have man pages & bash completion. Otherwise, you can generate a man page with `bento --help-man`, and bash completion with `bento --completion-script-bash`.

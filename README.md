Bento is a service manager for OS X with a robust cli interface, and a light system tray UI. It’s designed for personal use, as opposed to init.d or upstart.

## Installing

* Using [Homebrew](http://brew.sh/): `brew tap heewa/tap && brew install bento`
* Building manually (assuming you have a Go environment set up): `go get -v github.com/heewa/bento`, update with `go get -u -v github.com/heewa/bento`. If just running `bento` doesn’t work after that, you might need to set add `$GOPATH/bin` to your `$PATH` env var.

## Running

Just try `bento` in the command line! You can get more detailed help for a specific command like `bento help <cmd>` or `bento <cmd> --help`.

### Start a new service

You can run any command as a new, temporary (unsaved) service like: `bento run memcached`. If it takes arguments, you might get away with just appending them, but if they're flags, bento will try to parse them itself, so separate them like: `bento run memcached -- -v`.

I find it useful to start off tailing output of new services to make sure I got it right: `bento run --tail redis-server`. You can hit `<ctrl-c>` to interrupt bento, and it won't affect the service.

To see the _stdout_ & _stderr_ of a service, use `bento tail`. Add a `-f` to follow the output, much like the unix `tail` command. Similarly, `-F` will follow the output through restarts.

### Managing services

See what services you have with: `bento list`. You can start & stop services: `bento start <name>` & `bento stop <name>`. You can remove finished (exitted) temporary services with `bento clean`, though they'll be auto-removed after some time anyway.

### Saving services

You can save services permanently in a simple yaml file at `~/.bento/services.yml`. The file should contain a list of service definitions like:

```yaml
- name: Redis
  program: redis-server
  restart-on-exit: true
- name: Memcache
  program: memcached
  args:
    - -v
  auto-start: true
```

After changing the file, reload the service configuration without restarting with: `bento reload`. If you're having trouble getting a service right, try running it as a temp service (`bento run --args cmd -- cmd-args`), then get a yaml config for it with `bento list -l` (long list).

#### Service Configuration Options

* `name`: (required) The name of the service. You'll specify this to manage the service on the cli.
* `program`: (required) A full path to the binary to run.
* `args`: A list of arguments to the program.
* `dir`: A path to a runtime dir for the program. It defaults to the home dir of the server's starting user.
* `env`: A map of environment variable names to values.
* `auto-start`: If true, this service will be automatically started by bento when it first runs.
* `restart-on-exit`: If true, bento will attempt to restart the service if it exits. However, `bento stop` will cause the service to stop until explicitly started again.

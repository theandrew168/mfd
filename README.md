# mfd
Utility for managing multi-file application deployments

## Usage

```
usage: mfd <command> [<args>]
commands:
  list        List available deployments
  deploy      Resolve, fetch, build, activate, and restart a revision
  rollback    Rollback to the previous deployment
  clean       Remove non-active deployments
  help        Show this help message
```

## Example

I can use `mfd` to "deploy" a specific version of this project (using the `mfd.toml` file in this repo):
```
mfd deploy cc9bb24537014b7f16c14e745b4c3279dd61964a
```

Tags and short hashes can be used as well (they will be resolved to full hashes prior to deployment):
```
mfd deploy cc9bb24
mfd deploy v0.0.1
```

Now that version has been fetched, built, and activated:
```console
$ mfd list
cc9bb24537014b7f16c14e745b4c3279dd61964a (active)
```

I can then deploy a different version of the project just as easily:
```
mfd deploy a171e61f03f36be6a8aa0b8eb9bcd37ef380aed6
```

Now the _new_ version is active:
```console
$ mfd list
a171e61f03f36be6a8aa0b8eb9bcd37ef380aed6 (active)
cc9bb24537014b7f16c14e745b4c3279dd61964a
```

So, how does this work under the hood?
Simple!
It's just a symlink:
```console
$ ls -l
active -> a171e61f03f36be6a8aa0b8eb9bcd37ef380aed6
a171e61f03f36be6a8aa0b8eb9bcd37ef380aed6
cc9bb24537014b7f16c14e745b4c3279dd61964a
```

As long as your systemd service has `WorkingDirectory` set to `/my/app/active`, it'll only ever see the "active" deployment.
Once a new version is deployed (fetch, build, and update the symlink), restarting the service will cause the new code to be live.
There are no risks of directory contamination or file race conditions with this approach.
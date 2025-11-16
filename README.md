# mfd
Utility for managing multi-file application deployments

## Usage

```
usage: mfd <command> [<args>]
commands:
  list        List available deployments
  deploy      Resolve, fetch, build, and activate a revision
  resolve     Resolve a revision to a deployment
  activate    Activate a deployment
  restart     Restart a deployment
  remove      Remove a deployment
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

## Concept

This initial brain dump comes from thinking about how I'd deploy a NodeJS web application (using something like NextJS or SvelteKit).

Since this is a NodeJS app, you can't really bundle / build it into a clean, single binary for deployment (like I do with Go).
Given that, what needs to happen?
I still want to use systemd.
The server will need to have NodeJS installed to run the app.
Prior to starting, we need to run both `npm install` and `npm run build`.
Actually running the app will run `npm run start` or perhaps simply `node build/`.

Naively, this approach has issues.
Once the app is running, how can a new version of the code be deployed, built, and started **without impacting the currently running app**?
This problem also exists for other non-single-binary ecosystems such as Python.
There, tools like [shiv](https://shiv.readthedocs.io/en/latest/) solve the problem by transparently unzipping the app into a unique directory prior to running.
That way, the individual files of different versions don't conflict with each other and cutover race conditions are avoided.

Can we do something similar?
Systemd doesn't have a native way to say "use this arbitrary working directory (controlled via a variable, maybe?)".
One possible solution to this that I really like is simply making the `WorkingDirectory` a symlink ([reference](https://unix.stackexchange.com/questions/242019/set-workingdirectory-using-a-variable/629958#629958)).
Then, that symlink can point to any given revision of the project available on the server.

How would deployments work using this approach?
We'd first have a separate directory for each revision of the app that has been deployed (`/usr/local/bin/fussy/384a283` for example).
There will then exist a symlink (something like `/usr/local/bin/fussy/active`) that points to whichever version of the app is currently active.

When deploying a new version, clone and checkout its code into a new directory that correponds to its commit hash.
Then, from this new version directory, run both of the "build" steps: `npm install` and `npm run build`.
Next, update the "active" symlink to point to the new version directly.
Lastly, restart the systemd service which will pick up the code from the new version.

This will switch to the new code while minimizing the amount of “race condition” time spent running the old process with the new files.
Does NodeJS / NextJS even read anything from the FS once the service is running?
Surely something will be read from the FS during execution, like static resources or maybe compiled JS / CSS resources?
Either way, this approach minimizes the risk (though it doesn’t completely eliminate it).

Actually, this might work completely as intended without the “old process, new files” race condition.
Since systemd checks `WorkingDirectory` at startup, the old files should still be used even after the symlink is swapped.
The new files won’t be picked up until the service restarts (which also starts a new process). This might be perfect!
# Docker apps

An app runs in Docker when it has a Dockerfile and no `run` command. You can also pick the runtime yourself in the **Build** card on Overview.

Docker has to be installed on the server. Moonshed talks to it through the Docker API (the unix socket on Linux, the named pipe on Windows) and never shells out to the `docker` command. If Docker is missing, the build fails with an error saying so.

## Building

Images are built with BuildKit, so features like `RUN --mount=type=cache` work. By default Moonshed builds `Dockerfile` at the root of the repository. Change that with a `build` block:

```yaml
build:
  dockerfile: docker/Dockerfile.prod   # path from the repo root
  folder: api                          # the build context; default the repo root
  stage: runtime                       # build only up to this stage
  args:
    NODE_ENV: production
```

The same settings are in the **Build** card. The build output goes to the deployment log, and a build can be killed like any other.

Engines without BuildKit, such as a Windows containers daemon, fall back to Docker's classic builder.

## Running

Each release becomes an image named `moonshed/<app>:<release-id>`, and the container is created with:

- the app's env vars plus `PORT`, `APP_NAME` and `RELEASE_ID`,
- the app's port published on the host (`<port>:<port>`),
- `persist` paths bind-mounted from the app's `shared/` folder,
- the `unless-stopped` restart policy.

Env vars are passed to Docker directly and never written to disk.

Containers keep running when Moonshed restarts. On startup it reattaches to them instead of creating them again.

## What moonshed.yaml does for Docker apps

The file is optional. When present, Moonshed reads `port`, `persist`, `health`, `restart_unhealthy` and `build` from it. The `pre`, `run` and `post` scripts are not used, because the Dockerfile's `RUN` and `CMD` or `ENTRYPOINT` replace them.

**Run as OS user** isn't available for Docker apps. The container's user comes from the Dockerfile.

## Switching runtimes

Switching an app between native and Docker in the **Build** card stops the app and drops its current release, since the other engine can't run it. The next deploy builds it for the new runtime.

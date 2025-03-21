# go-get

A simple, efficient tool that finds and installs the latest compatible version of a Go module based on your local Go version, instead of upgrading your Go version...

## Features

- Automatically determines the latest compatible version of a Go module
- Filters out pre-release and incompatible versions
- Concurrent version checking for faster performance
- Built-in caching to improve subsequent lookups
- Respects Go version requirements in module dependencies

## Installation

```bash
go install github.com/liuyuxuan0723/go-get@latest
```

## Usage

Basic usage:

```bash
go-get [module]
```

### Options

```
-v, --verbose        Enable verbose logging
-t, --timeout int    Set global timeout in seconds (0 means no timeout) (default 60)
-h, --help           Help for go-get
```

### Examples

```bash
# Get the latest compatible version of a module
go-get github.com/gin-gonic/gin

# Force refresh the cache for the latest information
go-get -r github.com/gin-gonic/gin

# Enable verbose logging to see detailed progress
go-get -v github.com/gin-gonic/gin
```

## How It Works

1. Determines your local Go version (from `go.mod` or `go version` command)
2. Fetches available versions of the requested module
3. Filters out pre-release versions and incompatible versions
4. Checks Go version requirements in each module version's go.mod
5. Installs the latest compatible version

## Cache

The tool maintains a cache at `~/.mod_cache.json` to store module version information and reduce network requests. This speeds up subsequent queries for the same modules.

Use the `-r, --refresh` flag to bypass the cache and fetch the latest information.

## Why Use This?

- **Compatibility**: Ensures you get a version that works with your Go version
- **Performance**: Concurrent version checking and caching for faster resolution
- **Simplicity**: One command to find and install the right version
- **Safety**: Avoids incompatible versions that would break your build
- **Time-saving**: No need to manually check version compatibility

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

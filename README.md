![Build & Test Status](https://github.com/thepudds/gomoderate/actions/workflows/test.yml/badge.svg)

# gomoderate - for a pleasant Bluesky experience

There are many people in the world -- you are not obligated to see them all in your ~~timeline~~ skyline.

gomoderate is a command-line utility that gives you more control over your Bluesky Social experience by helping you do automated bulk moderation.

## Features

- Automatically mute users based on the block list of one or more accounts you trust.
- Import lists of users to mute from files or trusted URLs.
- Mute named users individually or in bulk.

Also, all of that for **blocking** is coming soon (hopefully!).

Note that on the Bluesky platform:
- **Mutes are private** -- only you can see who you have muted (plus in theory the system admins)
- **Blocks are public** -- anyone in the world could see your own block list, though the mobile apps and web apps don't yet show this info

## Installation

Downloadable binary releases will be available eventually, but for now, to install gomoderate, make sure you have [Go](https://go.dev/dl/) installed on your system, then run:

```bash
go install github.com/thepudds/gomoderate@latest
```

That will download & compile the source code. By default, it will install to the `go/bin` directory in your home directory.

One approach to execute the program is:
* Open a terminal (`Mac Terminal` on macOS, `Command Prompt` on Windows, or your shell on Linux)
* Type `cd go/bin` to change directories
* Run a sample help command by typing `./gomoderate help` (macOS/Linux) or `gomoderate help` (Windows)

Alternatively, you can add `$HOME/go/bin` to your path.

## Usage

A simple example invocation that doesn't require authentication is asking who someone else has blocked:

```bash
gomoderate list blocks @user1.bsky.social
```

In that example, `list` and `blocks` are considered "commands". The general structure for gomoderate commands is:

```bash
gomoderate [auth options] command subcommand [command options] [command arguments]
```

Many (but not all) of the commands require authentication. Before using gomoderate, you should obtain an application key from the Bluesky web interface. Go to [https://staging.bsky.app/settings/app-passwords](https://staging.bsky.app/settings/app-passwords) and create an application key. Your application key will look similar to `xj5s-fqo6-rtfm-lsrt`. (For brevity, we use `xyz` in the examples below).

## Examples

Here are some more examples of how to use gomoderate:

### Mute users

Mute one or more specified users:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz mute users @user1.bsky.social @user2.bsky.social
```

Bulk muting of unpleasant accounts that were blocked by accounts you trust. Here, you trust the blocking decisions of `@trusted1` and `@trusted2` and apply their blocks to your account as mutes:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz mute from-user-blocks @trusted1.bsky.social @trusted2.bsky.social
```

Mute users from a file or URL:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz mute from-file users.txt
```

```bash
gomoderate --my-user @me.bsky.social --app-key xyz mute from-url https://example.com/a-trusted-list-of-users-to-mute.txt
```

### Block users (coming soon)

Block one or more specified users:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz block users @user1.bsky.social @user2.bsky.social
```

Block users based on the users blocked by accounts you trust:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz block from-user-blocks @trusted1.bsky.social @trusted2.bsky.social
```

Block users from a file:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz block from-file users.txt
```

Block users from a URL:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz block from-url https://example.com/a-list-of-trusted-users-to-block.txt
```

### List mutes or blocks

List all users muted by you:

```bash
gomoderate --my-user @me.bsky.social --app-key xyz list mutes
```

List all users blocked by a specified user:

```bash
gomoderate list blocks @user1.bsky.social
```

## trusted-unpleasant-user-list.txt

gomoderate effectively defines a very simple file format defined that lists DIDs and handles, which can then be shared via URL or as files. 

One way to create such a file is via the --verbose flag:

```
gomoderate list blocks --verbose @user1.bsky.social > trusted-unpleasant-user-list.txt
```

That file can be served from any web server, and then anyone in the world can:

```
gomoderate <auth-flags> mute from-url https://example.com/> trusted-unpleasant-user-list.txt
```

At which point the person who ran that `mute from-url` command will be muting based on whatever DIDs were in that file. When reading the file, gomoderate only examines the DIDs, which are more permanent.

## Contributing

Open source makes the world go around! PRs welcome.

If you are not a developer, you can still contribute by:
 * filing a bug report (for example, if you see an error message that you think someone might find confusing)
 * improving the README
 * answering someone's question

## License

gomoderate is released under the open source [Apache 2.0 license](LICENSE).

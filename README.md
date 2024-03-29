# twtlmirror

This script takes all tweets from a user’s home timeline on Twitter and posts them to one Mastodon account.
It makes use of the CW tag to display from which user the tweet originally came from.
Media entities like photos, gifs and videos are mirrored as well.

Example use case:
A few people I care about still use Twitter, but I don’t want to check Twitter separately, so I’ve created a private
and locked bot account on my own Mastodon instance to which this script mirrors my Twitter home timeline. This way I
can keep up with their Tweets without them even having a Mastodon account or using a mirror script themselves.
Rudimentary support to mirror favs and mentions back to Twitter is now implemented.

Previously this tool was written in Python3, you can still find it in the [history](https://github.com/buckket/twtlmirror/tree/d7a8d3dc5d398d3f8042051251619cb2a996c349).

![Example of a twtlmirror bot profile](mirror.png)

## Installation

### From source

```sh
go get -u github.com/buckket/twtlmirror
```

## Configuration

- Edit config.toml (Twitter and Mastodon API credentials)

## Usage

```sh
./twtlmirror -config config.toml
```

## Notes

- This script works best when used in combination with systemd timers or cron.
- The script creates a SQLite database (twtlmirror.db) in the current working directory to save the last processed id.
  Make sure it is able to do so, or it will process the same tweets over and over again.

## License

GNU GPLv3+

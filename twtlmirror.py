#!/usr/bin/env python3

import html
import logging

import mastodon
import requests
import twitter

import settings


def read_since_id():
    try:
        with open("since_id", "r") as id_file:
            since_id = int(id_file.read())
    except (IOError, ValueError):
        since_id = 0
    return since_id


def write_since_id(since_id):
    with open("since_id", "w") as id_file:
        id_file.write(str(since_id))


def main():
    logging.basicConfig(format="%(asctime)s - %(levelname)s - %(message)s", level=logging.INFO)

    tapi = twitter.Api(consumer_key=settings.TWITTER_APP_KEY,
                       consumer_secret=settings.TWITTER_APP_SECRET,
                       access_token_key=settings.TWITTER_OAUTH_TOKEN,
                       access_token_secret=settings.TWITTER_OAUTH_TOKEN_SECRET,
                       tweet_mode="extended")
    mapi = mastodon.Mastodon(client_id=settings.MASTODON_CLIENT_KEY,
                             client_secret=settings.MASTODON_CLIENT_SECRET,
                             access_token=settings.MASTODON_ACCESS_TOKEN,
                             api_base_url=settings.MASTODON_URL)

    since_id = read_since_id()
    logging.info("Using since_id: {}".format(since_id))

    timeline = tapi.GetHomeTimeline(count=20, since_id=since_id)
    if timeline:
        for status in reversed(timeline):
            logging.info("Working on status {} by {}".format(status.id, status.user.screen_name))
            if status.quoted_status or status.retweeted_status:
                logging.info("Skipping status because its either a quote or a retweet")
                since_id = status.id if status.id > since_id else since_id
                continue

            media_ids = []
            embedded_urls = []
            if status.media:
                for media in status.media:
                    logging.info("Downloading media element ({})".format(media.type))
                    try:
                        req = requests.get(media.media_url)
                    except requests.exceptions.RequestException as e:
                        logging.exception(e)
                        continue
                    if req:
                        try:
                            media_ids.append(mapi.media_post(media_file=req.content,
                                                             mime_type=req.headers["content-type"]))
                            embedded_urls.append(media.url)
                        except mastodon.MastodonError as e:
                            logging.exception(e)

            try:
                text = status.full_text
                for embedded_url in embedded_urls:
                    text = text.replace(embedded_url, "")
                text = html.unescape(text)
                mapi.status_post(status=text, media_ids=media_ids, visibility="private",
                                 spoiler_text="@{}".format(status.user.screen_name))
                logging.info("{}: {}".format(status.user.screen_name, status.full_text))
                since_id = status.id if status.id > since_id else since_id
            except mastodon.MastodonError as e:
                logging.exception(e)

    write_since_id(since_id)


if __name__ == '__main__':
    main()

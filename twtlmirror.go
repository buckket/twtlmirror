package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/mattn/go-mastodon"
	"github.com/microcosm-cc/bluemonday"
	"github.com/spf13/viper"
	"html"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ByID []anaconda.Tweet

func (t ByID) Len() int           { return len(t) }
func (t ByID) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t ByID) Less(i, j int) bool { return t[i].Id < t[j].Id }

type Env struct {
	db   Database
	tapi *anaconda.TwitterApi
	mapi *mastodon.Client
}

func (env *Env) MirrorTweets() {
	user, err := env.tapi.GetSelf(url.Values{})
	if err != nil {
		log.Fatal(err)
	}

	sinceID, err := env.db.GetLastestTweetID()
	if err != nil {
		log.Print(err)
		sinceID = 1
	}
	log.Printf("Using sinceID: %d", sinceID)

	values := url.Values{}
	values.Add("since_id", strconv.FormatInt(sinceID, 10))
	values.Add("exclude_replies", "true")
	values.Add("include_entities", "true")
	tl, err := env.tapi.GetHomeTimeline(values)
	if err != nil {
		log.Fatal(err)
	}
	sort.Sort(ByID(tl))

	for _, t := range tl {
		log.Printf("Working on status %d by %s", t.Id, t.User.ScreenName)
		if t.RetweetedStatus != nil || t.QuotedStatus != nil {
			log.Print("Skipping status because its either a quote or a retweet")
			if t.Id > sinceID {
				sinceID = t.Id
			}
			continue
		}

		tootID, err := env.db.GetStatusByTweetID(t.Id)
		if err == nil {
			log.Printf("Tweet %d already processed (%s)", t.Id, tootID)
			continue
		}

		text := t.FullText

		var attachments []mastodon.ID
		for _, media := range t.ExtendedEntities.Media {
			log.Printf("Downloading media element (%s)", media.Type)
			var mediaURL string
			switch media.Type {
			case "video":
				mediaURL = media.VideoInfo.Variants[0].Url
			default:
				mediaURL = media.Media_url_https
			}
			resp, err := http.Get(mediaURL)
			if err != nil {
				log.Print(err)
				continue
			}

			att, err := env.mapi.UploadMediaFromReader(context.Background(), resp.Body)
			if err != nil {
				log.Print(err)
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			attachments = append(attachments, att.ID)
			text = strings.ReplaceAll(text, media.Url, "")
		}

		for _, eurl := range t.Entities.Urls {
			text = strings.ReplaceAll(text, eurl.Url, eurl.Expanded_url)
		}

		toot := mastodon.Toot{Status: html.UnescapeString(text),
			SpoilerText: fmt.Sprintf("@%s", t.User.ScreenName),
			Visibility:  "private",
			MediaIDs:    attachments,
		}

		if t.InReplyToStatusID > 0 {
			tootID, err := env.db.GetStatusByTweetID(t.InReplyToStatusID)
			if err == nil {
				toot.InReplyToID = mastodon.ID(tootID)
			}
		}

		status, err := env.mapi.PostStatus(context.Background(), &toot)
		if err != nil {
			log.Print(err)
			continue
		}

		log.Printf("[%s] %s: %q", status.ID, t.User.ScreenName, strings.ReplaceAll(strings.TrimSpace(t.FullText), "\n", " "))
		err = env.db.InsertStatus(t.Id, string(status.ID))
		if err != nil {
			log.Print(err)
		}
	}

	fields := make([]mastodon.Field, 4)
	fields[0] = mastodon.Field{Name: "Last updated", Value: time.Now().Format("2006-01-02 15:04")}
	fields[1] = mastodon.Field{Name: "Following", Value: strconv.Itoa(user.FriendsCount)}
	fields[2] = mastodon.Field{Name: "Followers", Value: strconv.Itoa(user.FollowersCount)}
	fields[3] = mastodon.Field{Name: "Database:", Value: strconv.Itoa(env.db.CountStatus())}

	note := fmt.Sprintf("Twitter mirror of %s (%s)", user.Name, user.ScreenName)

	_, err = env.mapi.AccountUpdate(context.Background(), &mastodon.Profile{Note: &note, Fields: &fields})
	if err != nil {
		log.Fatal(err)
	}
}

func (env *Env) MirrorMastodonNotifications() {
	var pg mastodon.Pagination
	for {
		nots, err := env.mapi.GetNotifications(context.Background(), &pg)
		if err != nil {
			log.Fatal(err)
		}
		if len(nots) == 0 {
			break
		}

		for _, not := range nots {
			switch not.Type {
			case "favourite":
				tweetID, err := env.db.GetStatusByTootID(string(not.Status.ID))
				if err != nil {
					err = env.mapi.DismissNotification(context.Background(), not.ID)
					if err != nil {
						log.Print(err)
					}
					continue
				}
				_, err = env.tapi.Favorite(tweetID)
				if err != nil {
					log.Print(err)
				}
			case "mention":
				log.Printf("Ment: %v", not.Status.InReplyToID)
				mention, ok := not.Status.InReplyToID.(string)
				if !ok {
					continue
				}
				tweetID, err := env.db.GetStatusByTootID(mention)
				if err != nil {
					err = env.mapi.DismissNotification(context.Background(), not.ID)
					if err != nil {
						log.Print(err)
					}
					continue
				}
				p := bluemonday.StrictPolicy()
				text := p.Sanitize(not.Status.Content)
				text = strings.TrimPrefix(text, "@mirror")

				values := url.Values{}
				values.Add("in_reply_to_status_id", strconv.FormatInt(tweetID, 10))
				values.Add("auto_populate_reply_metadata", "true")
				_, err = env.tapi.PostTweet(p.Sanitize(text), values)
			}
			err = env.mapi.DismissNotification(context.Background(), not.ID)
			if err != nil {
				log.Print(err)
			}
		}
		if pg.MaxID == "" {
			break
		}
	}
}

func main() {
	env := Env{}

	configPtr := flag.String("config", "", "path to config file")
	flag.Parse()

	if len(*configPtr) > 0 {
		viper.SetConfigFile(*configPtr)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Print(err)
	}
	viper.AutomaticEnv()

	env.tapi = anaconda.NewTwitterApiWithCredentials(viper.GetString("TWITTER_ACCESS_TOKEN"),
		viper.GetString("TWITTER_ACCESS_TOKEN_SECRET"),
		viper.GetString("TWITTER_CONSUMER_KEY"),
		viper.GetString("TWITTER_CONSUMER_SECRET"))
	_, err = env.tapi.VerifyCredentials()
	if err != nil {
		log.Fatal(err)
	}

	env.mapi = mastodon.NewClient(&mastodon.Config{
		Server:       viper.GetString("MASTODON_URL"),
		ClientID:     viper.GetString("MASTODON_CLIENT_ID"),
		ClientSecret: viper.GetString("MASTODON_CLIENT_SECRET"),
		AccessToken:  viper.GetString("MASTODON_ACCESS_TOKEN"),
	})
	_, err = env.mapi.GetAccountCurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	env.db, err = NewDatabase("./twtlmirror.db")
	if err != nil {
		log.Fatal(err)
	}
	defer env.db.Close()

	if err := env.db.CreateTableStatus(); err != nil {
		log.Fatal(err)
	}

	env.MirrorTweets()
	env.MirrorMastodonNotifications()
}

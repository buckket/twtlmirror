package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/mattn/go-mastodon"
	"github.com/spf13/viper"
	"html"
	"io/ioutil"
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

func readSinceID() int64 {
	idFile, err := ioutil.ReadFile("since_id")
	if err != nil {
		log.Print(err)
		return 1
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(idFile)))
	if err != nil {
		log.Print(err)
		return 1
	}
	return int64(id)
}

func writeSinceID(id int64) {
	err := ioutil.WriteFile("since_id", []byte(strconv.FormatInt(id, 10)), 0644)
	if err != nil {
		log.Print(err)
	}
}

func main() {
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

	tapi := anaconda.NewTwitterApiWithCredentials(viper.GetString("TWITTER_ACCESS_TOKEN"),
		viper.GetString("TWITTER_ACCESS_TOKEN_SECRET"),
		viper.GetString("TWITTER_CONSUMER_KEY"),
		viper.GetString("TWITTER_CONSUMER_SECRET"))
	_, err = tapi.VerifyCredentials()
	if err != nil {
		log.Fatal(err)
	}

	mapi := mastodon.NewClient(&mastodon.Config{
		Server:       viper.GetString("MASTODON_URL"),
		ClientID:     viper.GetString("MASTODON_CLIENT_ID"),
		ClientSecret: viper.GetString("MASTODON_CLIENT_SECRET"),
		AccessToken:  viper.GetString("MASTODON_ACCESS_TOKEN"),
	})
	_, err = mapi.GetAccountCurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	sinceID := readSinceID()
	log.Printf("Using sinceID: %d", sinceID)

	values := url.Values{}
	values.Add("since_id", strconv.FormatInt(sinceID, 10))
	values.Add("exclude_replies", "true")
	values.Add("include_entities", "true")
	tl, err := tapi.GetHomeTimeline(values)
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

			att, err := mapi.UploadMediaFromReader(context.Background(), resp.Body)
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
		status, err := mapi.PostStatus(context.Background(), &toot)
		if err != nil {
			log.Print(err)
			continue
		}

		log.Printf("[%s] %s: %q", status.ID, t.User.ScreenName, strings.ReplaceAll(strings.TrimSpace(t.FullText), "\n", " "))
		if t.Id > sinceID {
			sinceID = t.Id
		}
	}

	writeSinceID(sinceID)

	user, err := tapi.GetSelf(url.Values{})
	if err != nil {
		log.Fatal(err)
	}

	fields := make([]mastodon.Field, 3)
	fields[0] = mastodon.Field{Name: "Last updated", Value: time.Now().Format("2006-01-02 15:04")}
	fields[1] = mastodon.Field{Name: "Following", Value: strconv.Itoa(user.FriendsCount)}
	fields[2] = mastodon.Field{Name: "Followers", Value: strconv.Itoa(user.FollowersCount)}

	note := fmt.Sprintf("Twitter mirror of %s (%s)", user.Name, user.ScreenName)

	_, err = mapi.AccountUpdate(context.Background(), &mastodon.Profile{Note: &note, Fields: &fields})
	if err != nil {
		log.Fatal(err)
	}
}

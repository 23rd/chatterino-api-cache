package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gorilla/mux"
)

type LinkResolverResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`

	Tooltip string `json:"tooltip,omitempty"`

	// Flag in the BTTV API to.. maybe signify that the link will download something? idk
	// Download *bool  `json:"download,omitempty"`
}

var noLinkInfoFound = &LinkResolverResponse{
	Status:  404,
	Message: "No link info found",
}

var invalidURL = &LinkResolverResponse{
	Status:  500,
	Message: "Invalid URL",
}

func unescapeURLArgument(r *http.Request, key string) (string, error) {
	vars := mux.Vars(r)
	escapedURL := vars[key]
	url, err := url.PathUnescape(escapedURL)
	if err != nil {
		return "", err
	}

	return url, nil
}

func linkResolver(w http.ResponseWriter, r *http.Request) {
	url, err := unescapeURLArgument(r, "url")
	if err != nil {
		bytes, err := json.Marshal(invalidURL)
		if err != nil {
			fmt.Println("Error marshalling invalidURL struct:", err)
			return
		}
		_, err = w.Write(bytes)
		if err != nil {
			fmt.Println("Error in w.Write:", err)
		}
		return
	}

	response := cacheGetOrSet("url:"+url, 10*time.Minute, func() (interface{}, error) {
		resp, err := client.Get(url)
		if err != nil {
			if strings.HasSuffix(err.Error(), "no such host") {
				return json.Marshal(noLinkInfoFound)
			}

			return json.Marshal(&LinkResolverResponse{Status: 500, Message: "client.Get " + err.Error()})
		}

		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			doc, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				return json.Marshal(&LinkResolverResponse{Status: 500, Message: "html parser error " + err.Error()})
			}
			if strings.HasSuffix(resp.Request.URL.Host, ".youtube.com") {
				// do special youtube parsing

				queryValues := resp.Request.URL.Query()
				videoID := queryValues.Get("v")

				if videoID == "" {
					return json.Marshal(noLinkInfoFound)
				}

				youtubeResponse := cacheGetOrSet("youtube:"+videoID, 1*time.Hour, func() (interface{}, error) {
					video, err := getYoutubeVideo(videoID)
					if err != nil {
						return &LinkResolverResponse{Status: 500, Message: "youtube api error " + err.Error()}, nil
					}

					fmt.Println("Doing YouTube API Request on", videoID)
					return &LinkResolverResponse{
						Status:  resp.StatusCode,
						Tooltip: "<div style=\"text-align: left;\"><b>" + video.Snippet.Title + "</b><hr><b>Channel:</b> " + video.Snippet.ChannelTitle + "<br><b>Duration:</b> " + video.ContentDetails.Duration + "<br><b>Views:</b> " + strconv.FormatUint(video.Statistics.ViewCount, 10) + "<br><b>Likes:</b> <span style=\"color: green;\">+" + strconv.FormatUint(video.Statistics.LikeCount, 10) + "</span>/<span style=\"color: red;\">-" + strconv.FormatUint(video.Statistics.DislikeCount, 10) + "</span></div>",
					}, nil
				})

				return json.Marshal(youtubeResponse)
			}

			title := doc.Find("title").First().Text()
			return json.Marshal(&LinkResolverResponse{
				Status:  resp.StatusCode,
				Tooltip: fmt.Sprintf("<div style=\"text-align: left;\"><b>%s</b><hr><b>URL:</b> %s</div>", title, resp.Request.URL.String()),
			})
		}

		return json.Marshal(noLinkInfoFound)
	})

	_, err = w.Write(response.([]byte))
	if err != nil {
		fmt.Println("Error in w.Write:", err)
	}
}
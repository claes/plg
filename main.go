package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	url2 "net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	strip "github.com/grokify/html-strip-tags-go"
	"github.com/mmcdole/gofeed"
)

type Dms struct {
	Title     string        `json:"Title"`
	Resources []DmsResource `json:"Resources"`
}

type DmsResource struct {
	MimeType string `json:"MimeType"`
	Command  string `json:"Command"`
}

type PlaylistItem struct {
	title        string
	sorttitle    string
	description  string
	author       string
	iconUrl      string
	url          string
	id           string
	recursiveUrl string
	time         time.Time
}

var channels map[string]string

func main() {

	var destinationDir = flag.String("destination", ".", "Destination directory")
	var stanza = flag.String("stanza", "stanzas.txt", "Stanzas text file")
	//var stanza = flag.String("age", "0", "Age of files to keep")
	flag.Parse()

	channels = make(map[string]string)
	parseStanzas(*stanza, *destinationDir)

}

func parseStanzas(filename string, destinationDir string) {

	fmt.Printf("Parsing stanza file %s", filename)

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		fmt.Println(scanner.Text())
	}
	for i, line := range lines {
		if strings.HasPrefix(line, "http") {
			if i > 0 {
				title := lines[i-1]
				url := lines[i]

				filenameRegex := regexp.MustCompile(`(.*).txt`)
				filenameMatches := filenameRegex.FindStringSubmatch(path.Base(file.Name()))
				if len(filenameMatches) > 0 {

					svtRegex := regexp.MustCompile(`www.svtplay.se\/(.*)\/rss\.xml`)
					svtMatches := svtRegex.FindStringSubmatch(url)
					channelRegex := regexp.MustCompile(`youtube.com\/channel\/(.*)`)
					channelMatches := channelRegex.FindStringSubmatch(url)
					userRegex := regexp.MustCompile(`youtube.com\/user\/(.*)`)
					userMatches := userRegex.FindStringSubmatch(url)
					playlistRegex := regexp.MustCompile(`youtube.com\/playlist\?list=(.*)`)
					playlistMatches := playlistRegex.FindStringSubmatch(url)
					cRegex := regexp.MustCompile(`youtube.com\/c\/(.*)`) //Doesn't work. Need a way to figure out channel id in this case
					cMatches := cRegex.FindStringSubmatch(url)
					// /itemprop="channelId" content="(.*?)"/ and print $1
					title = strings.Trim(title, " .")
					filename := filenameMatches[1]
					if len(svtMatches) > 0 {
						svtCategory := svtMatches[1]
						parseAndWritePlaylists(title, fmt.Sprintf("https://www.svtplay.se/%s/rss.xml", svtCategory), destinationDir, filename)
					} else if len(channelMatches) > 0 {
						channelID := channelMatches[1]
						parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID), destinationDir, filename)
					} else if len(cMatches) > 0 {
						channelID := cMatches[1]
						parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID), destinationDir, filename)
					} else if len(userMatches) > 0 {
						user := userMatches[1]
						parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?user=%s", user), destinationDir, filename)
					} else if len(playlistMatches) > 0 {
						playlist := playlistMatches[1]
						parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?playlist_id=%s", playlist), destinationDir, filename)
					} else {
						parseAndWritePlaylists(title, url, destinationDir, filename)
					}
				} else {
					fmt.Println("Filename must end with .txt")
					continue
				}

			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func parseAndWritePlaylists(title string, url string, destinationDir string, prefix string) error {
	fmt.Printf("%s %s \n\n", title, url)
	if len(url) > 0 {
		_, playlist := parseFeed(url)
		if playlist == nil {
			fmt.Println("Skipping playlist for: ", title)
			return nil
		} else {
			fmt.Println("Writing playlist for: ", title)
			err := writePlaylist(destinationDir, prefix, title, playlist)
			if err != nil {
				fmt.Errorf("Error while writing playlist for %v;  %v", playlist, err)
			}
		}

	}
	return nil
}

func parseFeed(url string) (string, []PlaylistItem) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		fmt.Errorf("Error while writing playlist for %s;  %v \n", url, err)
		return "", nil
	}
	if len(feed.Items) == 0 {
		fmt.Errorf("No items in feed for  %s \n", url)
		return "", nil
	}
	fmt.Printf("Parsing feed for %s %s\n\n", feed.Title, url)

	if feed.Link != "" {
		channelRegex := regexp.MustCompile(`www.youtube.com\/channel\/(.*)`)
		channelMatches := channelRegex.FindStringSubmatch(feed.Link)
		if len(channelMatches) > 0 {
			channelId := channelMatches[1]
			fmt.Printf("Channel: %s ", channelId)
			channels[channelId] = feed.Title
		}

		/*
			channelRegex = regexp.MustCompile(`www.svtplay.se/(.*)`)
			channelMatches = channelRegex.FindStringSubmatch(feed.Link)
			if len(channelMatches) > 0 {
				channelId := channelMatches[1]
				fmt.Printf("Channel: %s ", channelId)
				channels[channelId] = feed.Title
			}
		*/

	}

	playlist := make([]PlaylistItem, 0)
	for _, item := range feed.Items {

		fmt.Printf("Item: %s ; %s ; %s ; %s ; %v", item.PublishedParsed, item.UpdatedParsed, item.Published, item.Updated, item)

		url := ""
		id := ""
		r := regexp.MustCompile(`www.youtube.com\/watch\?v=(.*)`)
		matches := r.FindStringSubmatch(item.Link)
		if len(matches) > 0 {
			id = matches[1]
			url = "plugin://plugin.video.youtube/play/?video_id=" + id
		} else {
			r := regexp.MustCompile(`www.svtplay.se(\/.*)`)
			matches := r.FindStringSubmatch(item.Link)
			if len(matches) > 0 {
				url = "plugin://plugin.video.svtplay/?mode=video&id=" + url2.QueryEscape(matches[1])
			}
		}
		if len(url) == 0 {
			continue
		}

		//url :=
		imageUrl := ""
		if feed.Image != nil {
			imageUrl = feed.Image.URL
		}
		if item.Image != nil {
			imageUrl = item.Image.URL
		}
		if len(imageUrl) < 1 {
			imageUrl = getImageUrl(*item)
		}

		title := strip.StripTags(item.Title)

		sorttitle := item.Updated + " " + title

		description := item.Description
		if len(description) < 1 {
			description = getDescription(*item)
		}
		description = strip.StripTags(description)
		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		time := time.Now()
		if item.PublishedParsed != nil {
			time = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			time = *item.UpdatedParsed
		}

		playlistItem := PlaylistItem{title, sorttitle, description, author, imageUrl, url, id, item.Link, time}
		playlist = append(playlist, playlistItem)
		fmt.Printf("%s %s \n", playlistItem.title, playlistItem.url)
	}
	return feed.Title, playlist
}

func getDescription(item gofeed.Item) string {
	//fmt.Printf("-- %s -- \n", item.Extensions["media"]["group"][0].Children["description"][0].Value)
	a := item.Extensions["media"]["group"]
	if len(a) > 0 {
		a := a[0].Children["description"]
		if len(a) > 0 {
			return a[0].Value
		}
	}
	return ""
}

func getImageUrl(item gofeed.Item) string {
	//fmt.Printf("-- %s -- \n", item.Extensions["media"]["group"][0].Children["description"][0].Value)
	a := item.Extensions["media"]["group"]
	if len(a) > 0 {
		a := a[0].Children["thumbnail"]
		if len(a) > 0 {
			return a[0].Attrs["url"]
		}
	}
	if len(item.Enclosures) > 0 {
		for _, enclosure := range item.Enclosures {
			if enclosure.Type == "image/jpeg" {
				return enclosure.URL
			}
		}
	}
	return ""
	//return item.Extensions["media"]["group"][0].Children["thumbnail"][0].Attrs["url"]
}

func writePlaylist(destinationDir string, prefix string, name string, playlist []PlaylistItem) error {

	n := name
	n = strings.Replace(n, "+", "", -1)
	n = strings.Replace(n, "/", "", -1)
	n = strings.Replace(n, "?", "", -1)
	n = strings.Replace(n, "|", "", -1)
	n = strings.Replace(n, ":", "", -1)

	dir := destinationDir + "/" + prefix + "/" + n + "/"
	fmt.Printf("Will create directory %s\n", dir)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}
	fmt.Printf("Created directory %s. Will now create %d playlist items.\n", dir, len(playlist))

	dirStat, err := os.Stat(dir)
	if err != nil {
		return err
	}
	dirTime := dirStat.ModTime()

	baseDir := destinationDir + "/" + prefix + "/"
	baseDirStat, err := os.Stat(baseDir)
	if err != nil {
		return err
	}
	baseDirTime := baseDirStat.ModTime()

	for _, item := range playlist {

		title := item.title
		title = strings.Replace(title, "+", "", -1)
		title = strings.Replace(title, "/", "", -1)
		title = strings.Replace(title, "?", "", -1)
		title = strings.Replace(title, "|", "", -1)
		title = strings.Replace(title, ":", "", -1)
		strmfile := dir + title + ".strm"
		nfofile := dir + title + ".nfo"
		dmsfile := dir + title + ".dms.json"

		{
			// Stream
			strm, err := os.Create(strmfile)
			if err != nil {
				return err
			}
			defer strm.Close()
			defer os.Chtimes(strmfile, item.time, item.time)

			w := bufio.NewWriter(strm)
			if err != nil {
				return err
			}
			_, err = w.WriteString(item.url + "\n")
			if err != nil {
				return err
			}
			w.Flush()

			//Info
			nfo, err := os.Create(nfofile)
			if err != nil {
				return err
			}
			defer nfo.Close()
			defer os.Chtimes(nfofile, item.time, item.time)

			w = bufio.NewWriter(nfo)
			if err != nil {
				return err
			}

			tag := strings.Replace(name, "&", "&amp;", -1)
			tag = strings.Replace(tag, "<", "&lt;", -1)
			tag = strings.Replace(tag, "<", "&gt;", -1)

			s := fmt.Sprintf("<?xml version='1.0' encoding='utf-8'?>\n<movie>\n<title>%s</title>\n<sorttitle>%s</sorttitle>\n<plot>%s</plot>\n<thumb>%s</thumb>\n<tag>%s</tag>\n</movie>\n",
				item.title, item.sorttitle, item.description, item.iconUrl, tag)
			_, err = w.WriteString(s)

			if err != nil {
				return err
			}
			w.Flush()

			//DMS
			dms, err := os.Create(dmsfile)
			if err != nil {
				return err
			}
			defer dms.Close()
			defer os.Chtimes(dmsfile, item.time, item.time)

			w = bufio.NewWriter(dms)
			if err != nil {
				return err
			}

			command := fmt.Sprintf("play-stream %s", item.id)
			jsonData, err := json.Marshal(&Dms{Title: item.title, Resources: []DmsResource{{MimeType: "video/mp4", Command: command}}})
			if err != nil {
				return err
			}

			_, err = dms.Write(jsonData)
			if err != nil {
				return err
			}
			w.Flush()

			if baseDirTime.Before(item.time) {
				baseDirTime = item.time
			}
			if dirTime.Before(item.time) {
				dirTime = item.time
			}
		}
		err = os.Chtimes(strmfile, item.time, item.time)
		if err != nil {
			fmt.Printf("Could not change mtime of %s: %v\n", strmfile, err)
		}
		err = os.Chtimes(nfofile, item.time, item.time)
		if err != nil {
			fmt.Printf("Could not change mtime of %s: %v\n", nfofile, err)
		}

		svtProgramRegex := regexp.MustCompile(`www.svtplay.se\/([^\/]+)$`)
		svtProgramRegexMatches := svtProgramRegex.FindStringSubmatch(item.recursiveUrl)
		if len(svtProgramRegexMatches) > 0 {
			svtProgram := svtProgramRegexMatches[1]
			programPrefix := prefix + "/" + n
			parseAndWritePlaylists(title, fmt.Sprintf("https://www.svtplay.se/%s/rss.xml", svtProgram), destinationDir, programPrefix)
		}
	}
	err = os.Chtimes(dir, dirTime, dirTime)
	if err != nil {
		fmt.Printf("Could not change mtime of %s: %v\n", dir, err)
	}
	err = os.Chtimes(baseDir, baseDirTime, baseDirTime)
	if err != nil {
		fmt.Printf("Could not change mtime of %s: %v\n", baseDir, err)
	}
	return nil
}

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"log/slog"
	"net/http"
	url2 "net/url"
	"os"
	"path"
	"regexp"
	"slices"
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
	url          string
	iconUrl      string
	strmUrl      string
	id           string
	recursiveUrl string
	time         time.Time
}

var channels map[string]string
var sleep *int

func main() {

	var destinationDir = flag.String("destination", ".", "Destination directory")
	var stanza = flag.String("stanza", "stanzas.txt", "Stanzas text file, or - for stdin")
	var name = flag.String("name", "", "Name to use. Required if stanza is stdin")
	var debug = flag.Bool("debug", false, "Debug logging")
	var parseChannelPlaylists = flag.Bool("channelPlaylists", false, "Parse channel playlists")
	sleep = flag.Int("sleep", 0, "Sleep between steps")

	//var stanza = flag.String("age", "0", "Age of files to keep")
	flag.Parse()

	if *debug {
		var programLevel = new(slog.LevelVar)
		programLevel.Set(slog.LevelDebug)
		handler := slog.NewTextHandler(os.Stderr,
			&slog.HandlerOptions{Level: programLevel})
		slog.SetDefault(slog.New(handler))
	}

	channels = make(map[string]string)
	parseStanzas(*stanza, *name, *destinationDir, *parseChannelPlaylists)
}

func parseStanzas(filename string, name string, destinationDir string, parseChannelPlaylists bool) {
	var file *os.File
	var err error
	defer file.Close()

	if filename == "-" {
		file = os.Stdin
		slog.Info("Parsing standard input")
	} else {
		file, err = os.Open(filename)
		if err != nil {
			log.Fatal(err)
		}
		slog.Info("Parsing stanza file", "filename", filename)
	}

	prefix := ""
	if len(name) > 0 {
		prefix = name
	} else {
		filenameRegex := regexp.MustCompile(`(.*).txt`)
		filenameMatches := filenameRegex.FindStringSubmatch(path.Base(file.Name()))
		if len(filenameMatches) > 0 {
			prefix = filenameMatches[1]
		}
	}

	if prefix == "" {
		slog.Info("If name not given, filename must end with .txt", "filename", file.Name())
	} else {
		scanner := bufio.NewScanner(file)
		lines := make([]string, 0)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			slog.Debug("Scanned line", "line", scanner.Text())
		}

		playlistMap := createPlaylistMap(lines)
		parsePlaylists(playlistMap, destinationDir, prefix, parseChannelPlaylists)

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}
}

// Create a map from list of lines with title followed by url
func createPlaylistMap(lines []string) map[string]string {

	playlistMap := make(map[string]string)
	for i, line := range lines {
		if strings.HasPrefix(line, "http") {
			if i > 0 {
				title := lines[i-1]
				url := lines[i]
				playlistMap[url] = title
			}
		}
	}
	return playlistMap
}

func parsePlaylists(playlists map[string]string, destinationDir string, prefix string, parseChannelPlaylists bool) {
	for url, title := range playlists {
		parsePlaylist(title, url, destinationDir, prefix, parseChannelPlaylists)
		time.Sleep(time.Duration(*sleep) * time.Second)
	}
}

func parsePlaylist(title, url, destinationDir, prefix string, parseChannelPlaylists bool) {
	slog.Debug("Parsing playlist", "title", title, "prefix", prefix, "url", url)
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

	redditRegex := regexp.MustCompile(`reddit.com\/r\/([^/]+)`)
	redditMatches := redditRegex.FindStringSubmatch(url)

	// /itemprop="channelId" content="(.*?)"/ and print $1
	title = strings.Trim(title, " .")

	if len(svtMatches) > 0 {
		svtCategory := svtMatches[1]
		slog.Debug("SVT category detected", "category", svtCategory)
		parseAndWritePlaylists(title, fmt.Sprintf("https://www.svtplay.se/%s/rss.xml", svtCategory), destinationDir, prefix)
	} else if len(channelMatches) > 0 {
		channelID := channelMatches[1]
		slog.Debug("YouTube channel detected", "channel", channelID)
		parseAndWriteChannelPlaylists(url, channelID, parseChannelPlaylists, title, destinationDir, prefix)
	} else if len(cMatches) > 0 {
		channelID := cMatches[1]
		slog.Debug("YouTube c channel detected", "channel", channelID)
		parseAndWriteChannelPlaylists(url, channelID, parseChannelPlaylists, title, destinationDir, prefix)
	} else if len(userMatches) > 0 {
		user := userMatches[1]
		slog.Debug("YouTube user detected", "user", user)
		parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?user=%s", user), destinationDir, prefix)
	} else if len(playlistMatches) > 0 {
		playlist := playlistMatches[1]
		slog.Debug("YouTube playlist detected", "playlist", playlist)
		parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?playlist_id=%s", playlist), destinationDir, prefix)
	} else if len(redditMatches) > 0 {
		subreddit := redditMatches[1]
		slog.Debug("Subreddit detected", "subreddit", subreddit)
		parseAndWritePlaylists(title, fmt.Sprintf("https://www.reddit.com/r/%s/.rss", subreddit), destinationDir, prefix)
	} else {
		parseAndWritePlaylists(title, url, destinationDir, prefix)
	}
}

func parseAndWriteChannelPlaylists(url string, channelID string, parseChannelPlaylists bool, title string, destinationDir string, prefix string) {

	fragmentRegex := regexp.MustCompile(`#([^#]+)$`)
	match := fragmentRegex.FindStringSubmatch(url)

	parseVideos := true
	fragment := ""
	if len(match) > 1 {
		fragment = match[1]
	}
	slog.Debug("Parsing channel playlists", "title", title)

	if parseChannelPlaylists || strings.Contains(fragment, "p") { //playlists
		playlistRegex := "\"playlistId\":\"(PL[a-zA-Z0-9_-]{16,32})\""
		playlistsSection := "playlists"
		playlistsExisted := parseAndWriteChannelPlaylistsForSection(channelID, destinationDir, prefix, title, playlistsSection, playlistRegex)
		parseVideos = parseVideos && !playlistsExisted
	}
	if parseChannelPlaylists || strings.Contains(fragment, "r") { //releases
		releasesRegex := "\"playlistId\":\"(OL[a-zA-Z0-9_-]{39})\""
		releasesSection := "releases"
		releasesExisted := parseAndWriteChannelPlaylistsForSection(channelID, destinationDir, prefix, title, releasesSection, releasesRegex)
		parseVideos = parseVideos && !releasesExisted
	}

	if parseVideos {
		parseAndWritePlaylists(title, fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID), destinationDir, prefix)
	}
}

func parseAndWriteChannelPlaylistsForSection(channelID string, destinationDir string, prefix string, title string,
	section string, playlistRegex string) bool {
	playlistIds := getYoutubePlaylistsForChannel(channelID, section, playlistRegex)
	playlistMap := make(map[string]string)
	for _, playlistId := range playlistIds {
		playlistName := getYoutubePlaylistName(playlistId)
		if len(playlistName) < 1 {
			playlistName = playlistId
		}
		playlistURL := "https://www.youtube.com/playlist?list=" + playlistId
		playlistMap[playlistURL] = playlistName

		time.Sleep(time.Duration(*sleep) * time.Second)
	}
	if len(playlistMap) > 0 {
		parsePlaylists(playlistMap, destinationDir+"/"+prefix+"/"+title, section, false)
		return true
	} else {
		return false
	}
}

func getYoutubePlaylistsForChannel(channelId string, section string, extractionRegex string) []string {
	re, err := regexp.Compile(extractionRegex)
	if err != nil {
		slog.Error("Error compiling regex", "regex", re, "error", err)
		return nil
	}

	channelPlaylistsUrl := "https://www.youtube.com/channel/" + channelId + "/" + section
	resp, err := http.Get(channelPlaylistsUrl)
	if err != nil {
		slog.Error("Error fetching URL", "url", channelPlaylistsUrl, "error", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Error reading response body for url", "url", channelPlaylistsUrl, "error", err)
		return nil
	}

	var playlistIds []string
	matches := re.FindAllStringSubmatch(string(body), -1)
	for _, match := range matches {
		if len(match) > 1 {
			playlistIds = append(playlistIds, match[1])
		}
	}

	slices.Sort(playlistIds)
	return slices.Compact(playlistIds)
}

func getYoutubePlaylistName(playlistId string) string {
	titleRegex := "<title>(.*?)(?:- YouTube)?</title>"
	re, err := regexp.Compile(titleRegex)
	if err != nil {
		slog.Error("Error compiling regex", "regex", re, "error", err)
		return ""
	}

	playlistUrl := "https://www.youtube.com/playlist?list=" + playlistId
	resp, err := http.Get(playlistUrl)
	if err != nil {
		slog.Error("Error fetching URL", "url", playlistUrl, "error", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Error reading response body for url", "url", playlistUrl, "error", err)
		return ""
	}

	matches := re.FindAllStringSubmatch(string(body), -1)
	for _, match := range matches {
		if len(match) > 1 {
			return html.UnescapeString(match[1])
		} else {
			slog.Error("Could not find title match for playlist", "url", playlistUrl)
		}
	}
	slog.Error("Could not find title match for playlist, returning", "url", playlistUrl)
	return ""
}

func parseAndWritePlaylists(title string, url string, destinationDir string, prefix string) error {
	slog.Info("Parsing playlist", "title", title, "url", url)
	if len(url) > 0 {
		_, playlist := parseFeed(url)
		if playlist == nil {
			slog.Debug("Skipping playlist", "title", title)
			return nil
		} else {
			slog.Debug("Writing playlist", "title", title)
			err := writePlaylist(destinationDir, prefix, title, playlist)
			if err != nil {
				slog.Error("Error writing playlist", "playlist", playlist, "title", title, "error", err)
				//fmt.Errorf("Error while writing playlist for %v;  %v", playlist, err)
			}
		}
	}
	return nil
}

func parseFeed(url string) (string, []PlaylistItem) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		slog.Error("Error writing playlist", "url", url, "error", err)
		//fmt.Errorf("Error while writing playlist for %s;  %v \n", url, err)
		return "", nil
	}
	if len(feed.Items) == 0 {
		slog.Error("No items in feed", "url", url)
		//fmt.Errorf("No items in feed for  %s \n", url)
		return "", nil
	}
	slog.Debug("Parsing feed", "title", feed.Title, "url", url)
	if feed.Link != "" {
		channelRegex := regexp.MustCompile(`www.youtube.com\/channel\/(.*)`)
		channelMatches := channelRegex.FindStringSubmatch(feed.Link)
		if len(channelMatches) > 0 {
			channelId := channelMatches[1]
			slog.Debug("Adding channel", "channel", channelId)
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

		slog.Debug("Processing playlist item", "publishedParsed", item.PublishedParsed, "updatedParsed", item.UpdatedParsed, "published",
			item.Published, "updated", item.Updated, "item", item)
		//fmt.Printf("Item: %s ; %s ; %s ; %s ; %v", item.PublishedParsed, item.UpdatedParsed, item.Published, item.Updated, item)

		strmUrl := ""
		id := ""
		url := ""
		r := regexp.MustCompile(`www.youtube.com\/watch\?v=(.*)`)
		matches := r.FindStringSubmatch(item.Link)
		if len(matches) > 0 {
			id = matches[1]
			url = item.Link
			strmUrl = "plugin://plugin.video.youtube/play/?video_id=" + id
		}

		if len(strmUrl) == 0 {
			r := regexp.MustCompile(`www.svtplay.se(\/.*)`)
			matches := r.FindStringSubmatch(item.Link)
			url = item.Link
			if len(matches) > 0 {
				strmUrl = "plugin://plugin.video.svtplay/?mode=video&id=" + url2.QueryEscape(matches[1])
			}
		}

		// For example from Reddit feed, the contents is not item.Link but in item.Content
		if len(strmUrl) == 0 {
			r := regexp.MustCompile(`youtube.com\/watch\?v=([a-zA-Z0-9-_]{11})`)
			matches := r.FindStringSubmatch(item.Content)
			if len(matches) > 0 {
				id = matches[1]
				url = "https://www.youtube.com/watch?v=" + id
				strmUrl = "plugin://plugin.video.youtube/play/?video_id=" + id
			}
		}

		if len(strmUrl) == 0 {
			r := regexp.MustCompile(`youtu.b\/([a-zA-Z0-9-_]{11})`)
			matches := r.FindStringSubmatch(item.Content)
			if len(matches) > 0 {
				id = matches[1]
				url = "https://www.youtube.com/watch?v=" + id
				strmUrl = "plugin://plugin.video.youtube/play/?video_id=" + id
			}
		}

		if len(strmUrl) == 0 {
			continue
		}

		//url :=
		slog.Debug("Getting image")
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

		playlistItem := PlaylistItem{title, sorttitle, description, author, url, imageUrl, strmUrl, id, item.Link, time}
		playlist = append(playlist, playlistItem)
		slog.Debug("Created playlist item", "title", playlistItem.title, "url", playlistItem.url, "strmUrl", playlistItem.strmUrl)
		//fmt.Printf("%s %s \n", playlistItem.title, playlistItem.url)
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
	//fmt.Printf("-- %s -- \n", item.Extensions["media"]["thumbnail"][0].Attrs["url"])

	//a := item.Extensions["media"]["group"]
	if mediaMap, ok := item.Extensions["media"]; ok {
		if group, ok := mediaMap["group"]; ok {
			a := group[0].Children["thumbnail"]
			if len(a) > 0 {
				return a[0].Attrs["url"]
			}
		}
	}

	//a = item.Extensions["media"]["thumbnail"]
	if mediaMap, ok := item.Extensions["media"]; ok {
		if thumbnail, ok := mediaMap["thumbnail"]; ok {
			if len(thumbnail) > 0 {
				return thumbnail[0].Attrs["url"]
			}
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
	slog.Debug("Will create directory", "directory", dir)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}
	epoch := time.Unix(1, 0)
	err = os.Chtimes(dir, epoch, epoch) // Set both atime and mtime
	if err != nil {
		return err
	}
	slog.Info("Created directory, will now create playlist items", "directory", dir, "noOfItems", len(playlist))

	var mostRecentTime time.Time
	baseDir := destinationDir + "/" + prefix + "/"

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

			// URL. Disable for now
			// urlfile := dir + title + ".url"
			// url, err := os.Create(urlfile)
			// if err != nil {
			// 	return err
			// }
			// defer url.Close()
			// defer os.Chtimes(urlfile, item.time, item.time)

			// w := bufio.NewWriter(url)
			// if err != nil {
			// 	return err
			// }
			// _, err = w.WriteString(item.url)
			// if err != nil {
			// 	return err
			// }
			// w.Flush()

			// Stream
			strm, err := os.Create(strmfile)
			if err != nil {
				return err
			}
			defer strm.Close()
			defer os.Chtimes(strmfile, item.time, item.time)

			var w = bufio.NewWriter(strm)
			if err != nil {
				return err
			}
			_, err = w.WriteString(item.strmUrl + "\n")
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

			if mostRecentTime.IsZero() || mostRecentTime.Before(item.time) {
				mostRecentTime = item.time
			}
		}
		err = os.Chtimes(strmfile, item.time, item.time)
		if err != nil {
			slog.Error("Could not change mtime of strm file", "file", strmfile, "error", err)
		}
		err = os.Chtimes(nfofile, item.time, item.time)
		if err != nil {
			slog.Error("Could not change mtime of nfo file", "file", nfofile, "error", err)
		}

		svtProgramRegex := regexp.MustCompile(`www.svtplay.se\/([^\/]+)$`)
		svtProgramRegexMatches := svtProgramRegex.FindStringSubmatch(item.recursiveUrl)
		if len(svtProgramRegexMatches) > 0 {
			svtProgram := svtProgramRegexMatches[1]
			programPrefix := prefix + "/" + n
			parseAndWritePlaylists(title, fmt.Sprintf("https://www.svtplay.se/%s/rss.xml", svtProgram), destinationDir, programPrefix)
		}
	}

	err = os.Chtimes(dir, mostRecentTime, mostRecentTime)

	baseDirStat, err := os.Stat(baseDir)
	if err != nil {
		return err
	}
	if baseDirStat.ModTime().Before(mostRecentTime) {
		err = os.Chtimes(baseDir, mostRecentTime, mostRecentTime)
		if err != nil {
			slog.Error("Could not change mtime of basedir", "directory", baseDir, "error", err)
		}
	}
	return nil
}

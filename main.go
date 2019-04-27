package main

import (
	"crypto/tls"
	"flag"
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"github.com/t-tomalak/logrus-easy-formatter"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

var games = make(map[string][]Game)

type Game struct {
	Date     time.Time
	DateStr  string
	Opponent string
}

type Summary struct {
	Team      string
	ExtraDays int
}

func main() {
	flag.Parse()

	logLevel := flag.String("loglevel", "info", "log level, support debug,info,warn,error")

	logrus.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "%msg%\n",
	})
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		panic(err.Error())
	}
	logrus.SetLevel(level)

	// Set up HTTP client that ignores the broken cert on fogis.se
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}
	client := &http.Client{Transport: transCfg}

	// Load the HTML of the fixture list from fogis
	res, err := client.Get("https://fogis.se/information/?scr=fixturelist&ftid=77486")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		logrus.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Find the games using some CSS selectors
	doc.Find(".clTrOdd, .clTrEven").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		date := s.Find(".matchTid span").Text()
		d, err := time.Parse("2006-01-02 15:04", date)

		if err != nil {
			d, err = time.Parse("2006-01-02", date)
			if err != nil {
				panic(err.Error())
			}
		}

		// Normalize dates to midnight to eliminate differences in time of day.
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())

		teams := s.Find("td:not([nowrap]) a[href*=\"result\"]").Text()
		homeTeam := strings.Split(teams, " - ")[0]
		awayTeam := strings.Split(teams, " - ")[1]

		if games[homeTeam] == nil {
			games[homeTeam] = make([]Game, 0)
		}
		if games[awayTeam] == nil {
			games[awayTeam] = make([]Game, 0)
		}
		games[homeTeam] = append(games[homeTeam], Game{Date: d, DateStr: date, Opponent: awayTeam})
		games[awayTeam] = append(games[awayTeam], Game{Date: d, DateStr: date, Opponent: homeTeam})

	})

	// Extra list of teams
	teams := make([]string, 0)
	for k, _ := range games {
		teams = append(teams, k)
	}

	// Sort alphabetically ASC
	sort.Slice(teams, func(i, j int) bool {
		return teams[i] < teams[j]
	})

	// defines last date of spring season
	lastGameOfSpringSeason, _ := time.Parse("2006-01-02", "2019-06-10")

	totalDiff := 0

	totalList := make([]Summary, 0)

	// Iterate over all teams...
	for _, ownTeam := range teams {

		// for every game of every team, check each game and count days since lastGameOfSpringSeason game for themselves and opponent.
		var daysDiff = 0
		var gamesWithLessDays = 0
		var gamesWithMoreDays = 0

		ownGames := games[ownTeam]
		for idx, game := range ownGames {
			if idx == 0 || game.Date.After(lastGameOfSpringSeason) {
				continue
			}
			opp := game.Opponent

			logrus.Debugf("---------------------------\n%v spelade %v mot %v.\n", game.DateStr, ownTeam, opp)

			oppList := games[opp]
			lastGameIdx := -1
			// find this game for the opponent
			for oppIdx, oppGame := range oppList {
				if oppGame.DateStr == game.DateStr {
					lastGameIdx = oppIdx - 1
					break
				}
			}
			if lastGameIdx >= 0 {
				lastOppGame := oppList[lastGameIdx]
				logrus.Debugf("Senaste matchen för %v var mot %v %v", opp, lastOppGame.Opponent, lastOppGame.DateStr)
				logrus.Debugf("Senaste matchen för %v var mot %v %v", ownTeam, ownGames[idx-1].Opponent, ownGames[idx-1].DateStr)
				d1 := ownGames[idx-1].Date
				d2 := lastOppGame.Date
				d1diff := int(game.Date.Sub(d1).Hours() / 24)
				d2diff := int(game.Date.Sub(d2).Hours() / 24)

				logrus.Debugf("%v hade sin senaste match för %v dagar sedan.", ownTeam, d1diff)
				logrus.Debugf("%v hade sin senaste match för %v dagar sedan.", opp, d2diff)

				// if either team had 4 vilodagar or less, we include this game
				if d1diff < 5 || d2diff < 5 {
					daysDiff = daysDiff + d2diff - d1diff

					if d2diff > d1diff {
						gamesWithLessDays++
					}
					if d1diff > d2diff {
						gamesWithMoreDays++
					}
				}
			}
		}

		totalDiff += daysDiff

		if daysDiff < 0 {
			daysDiff = daysDiff * -1
			logrus.Infof("%v har totalt %v FLER vilodagar", ownTeam, daysDiff)
			totalList = append(totalList, Summary{Team: ownTeam, ExtraDays: daysDiff})

		} else if daysDiff > 0 {
			logrus.Infof("%v har totalt %v FÄRRE vilodagar", ownTeam, daysDiff)
			totalList = append(totalList, Summary{Team: ownTeam, ExtraDays: daysDiff * -1})

		} else {
			logrus.Infof("%v hade (+-) 0 vilodagar", ownTeam)
		}
		logrus.Infof("%v har %v matcher med färre vilodagar än sin motståndare", ownTeam, gamesWithLessDays)
		logrus.Infof("%v har %v matcher med fler vilodagar än sin motståndare\n", ownTeam, gamesWithMoreDays)

	}

	logrus.Infof("total difference borde vara 0, var %v", totalDiff)

	sort.Slice(totalList, func(i, j int) bool {
		return totalList[i].ExtraDays > totalList[j].ExtraDays
	})

	for _, t := range totalList {
		logrus.Infof("%v %v", t.Team, t.ExtraDays)
	}
}

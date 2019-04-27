package main

import (
	"demo-export/lib"
	"encoding/json"
	"fmt"
	dem "github.com/markus-wa/demoinfocs-golang"
	"github.com/markus-wa/demoinfocs-golang/common"
	"github.com/markus-wa/demoinfocs-golang/events"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Round struct {
	RoundNumber   int    `json:"round_number"`
	StartTick     int    `json:"start_tick"`
	UnfreezeTick  int    `json:"unfreeze_tick"`
	EndTick       int    `json:"end_tick"`
	BombExploded  bool   `json:"bomb_exploded"`
	BombDefused   bool   `json:"bomb_defused"`
	BombPlanted   bool   `json:"bomb_planted"`
	Ace           bool   `json:"ace"`
	AceBy         string   `json:"ace_by"`
	CtScore       int    `json:"ct_score"`
	TScore        int    `json:"t_score"`
	CtKills       int    `json:"ct_kills"`
	TKills        int    `json:"t_kills"`
	Duration      int    `json:"duration"`
	CutDuration   int    `json:"cut_duration"`
	Winner        string `json:"winner"`
	playing       bool
	lastTFragger  int
	tFragRow      int
	ctFragRow     int
	lastCtFragger int
}

type Game struct {
	Id             string            `json:"id"`
	Team1          string            `json:"team_1"`
	Team2          string            `json:"team_2"`
	MapName        string            `json:"map_name"`
	RoundsNumber   int               `json:"rounds_number"`
	TickRate       float64           `json:"tick_rate"`
	TickTime       float64           `json:"tick_time"`
	MaxTicks       int               `json:"max_ticks"`
	MaxTime        float64           `json:"max_time"`
	MatchStartTick int               `json:"match_start_tick"`
	MatchEndTick   int               `json:"match_end_tick"`
	Header         common.DemoHeader `json:"header"`
	Rounds         []Round           `json:"rounds"`
}

func main() {
	demos := readCurrentDir()
	if len(demos) == 0 {
		fmt.Println("There are no demos in the provided path")
		os.Exit(0)
	}
	sem := lib.NewSemaphore(3)
	for _, demo := range demos {
		sem.Add()
		go func(demo string) {
			defer sem.Done()
			processDemos(demo)
		}(demo)
	}
	sem.Wait()
	fmt.Println("Done")
}

func readCurrentDir() []string {
	path := os.Args[1]
	if path == "" {
		path = "."
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	pathExists, err := exists(path)
	if !pathExists || err != nil {
		panic("Path provided does not exist")
	}

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed opening directory: %s", err)
	}
	defer file.Close()

	var demos []string
	err = filepath.Walk(path, func(subPath string, f os.FileInfo, err error) error {
		if strings.HasSuffix(subPath, ".dem") {
			demos = append(demos, subPath)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("failed opening directory: %s", err)
	}

	return demos
}

func processDemos(demoFile string) {
	fmt.Printf("Starting to process %s\n", demoFile)
	baseDemoFile := strings.Replace(demoFile, ".dem", "", 1)
	f, err := os.Open(demoFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	p := dem.NewParser(f)
	header, err := p.ParseHeader()
	if err != nil {
		panic(err)
	}
	var game Game
	rounds := make([]Round, 0)
	var round Round
	game.Header = header
	game.MapName = header.MapName
	game.TickRate = header.TickRate()
	game.TickTime = header.TickTime().Seconds()
	game.MaxTicks = header.PlaybackTicks
	game.MaxTime = header.PlaybackTime.Seconds()
	game.Id = fmt.Sprintf("%s_%d%d", game.MapName, header.SignonLength, header.PlaybackTicks)

	// Register handler on kill events
	p.RegisterEventHandler(func(e events.IsWarmupPeriodChanged) {
		rounds = make([]Round, 0) // Reset the rounds
	})

	p.RegisterEventHandler(func(e events.ScoreUpdated) {
		if round.RoundNumber == 0 {
			return
		}
		rounds = append(rounds, round)
		round = Round{} // Restart it
	})

	p.RegisterEventHandler(func(e events.Kill) {
		if !round.playing {
			return
		}

		switch e.Victim.Team {
		case common.TeamTerrorists:
			round.CtKills++
			if round.lastTFragger == e.Killer.EntityID {
				round.tFragRow++
			}
			round.lastTFragger = e.Killer.EntityID
			round.tFragRow = 1
		case common.TeamCounterTerrorists:
			round.TKills++
			if round.lastCtFragger == e.Killer.EntityID {
				round.ctFragRow++
			}
			round.lastCtFragger = e.Killer.EntityID
			round.ctFragRow = 1
		}
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		gs := p.GameState()
		if !gs.IsMatchStarted() || gs.IsWarmupPeriod() {
			return
		}
		var tScore int
		var ctScore int
		round.Winner = e.WinnerState.ClanName
		switch e.Winner {
		case common.TeamTerrorists:
			// Winner's score + 1 because it hasn't actually been updated yet
			tScore, ctScore = gs.TeamTerrorists().Score+1, gs.TeamCounterTerrorists().Score
		case common.TeamCounterTerrorists:
			ctScore, tScore = gs.TeamCounterTerrorists().Score+1, gs.TeamTerrorists().Score
		default:
			// Probably match medic or something similar
			fmt.Println("Round finished: No winner (tie)")
			return
		}
		round.RoundNumber = ctScore + tScore
		round.CtScore = ctScore
		round.TScore = tScore
		round.EndTick = gs.IngameTick()
		round.playing = false
		round.Duration = int(float64(round.EndTick-round.StartTick) * game.TickTime)
		round.CutDuration = int(float64(round.EndTick-round.UnfreezeTick) * game.TickTime)
		round.Ace = round.tFragRow == 5 || round.ctFragRow == 5
		if round.Ace {
			fmt.Printf("There was an ace on %s\n", game.Id)
			if round.tFragRow == 5 && round.lastTFragger > 0 {
				round.AceBy = gs.Participants().FindByHandle(round.lastTFragger).Name
			}
			if round.ctFragRow == 5 && round.lastCtFragger > 0  {
				round.AceBy = gs.Participants().FindByHandle(round.lastCtFragger).Name
			}
		}
	})

	p.RegisterEventHandler(func(e events.MatchStart) {
		gs := p.GameState()
		game.MatchStartTick = gs.IngameTick()
		game.Team1 = gs.TeamCounterTerrorists().ClanName
		game.Team2 = gs.TeamTerrorists().ClanName
	})

	p.RegisterEventHandler(func(e events.MatchStartedChanged) {
		gs := p.GameState()
		game.MatchEndTick = gs.IngameTick()
	})

	p.RegisterEventHandler(func(e events.RoundStart) {
		gs := p.GameState()
		round.StartTick = gs.IngameTick()
		round.playing = true
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		gs := p.GameState()
		if !round.playing || gs.IngameTick() == round.EndTick {
			return
		}

		round.UnfreezeTick = gs.IngameTick()
	})

	p.RegisterEventHandler(func(e events.BombPlanted) {
		round.BombPlanted = true
	})

	p.RegisterEventHandler(func(e events.BombDefused) {
		round.BombDefused = true
	})

	p.RegisterEventHandler(func(e events.BombExplode) {
		round.BombExploded = true
	})

	// Parse to end
	err = p.ParseToEnd()

	if err != nil {
		panic(err)
	}
	game.Rounds = rounds
	game.RoundsNumber = len(rounds)
	jsonOutput, _ := json.MarshalIndent(game, "", "  ")
	err = ioutil.WriteFile(baseDemoFile+".json", jsonOutput, 0644)
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

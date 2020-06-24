package main

import (
	"demo-export/lib"
	"encoding/json"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	dem "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/events"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Round struct {
	RoundNumber     int    `json:"round_number"`
	StartTick       int    `json:"start_tick"`
	UnfreezeTick    int    `json:"unfreeze_tick"`
	OfficialEndTick int    `json:"official_end_tick"`
	EndTick         int    `json:"end_tick"`
	BombExploded    bool   `json:"bomb_exploded"`
	BombExplodedAt  int    `json:"bomb_exploded_at"`
	BombDefused     bool   `json:"bomb_defused"`
	BombDefusedAt   int    `json:"bomb_defused_at"`
	BombPlanted     bool   `json:"bomb_planted"`
	Ace             bool   `json:"ace"`
	AceBy           string `json:"ace_by"`

	CT                 string `json:"ct"`
	CTScore            int    `json:"ct_score"`
	PreviousCTScore    int    `json:"previous_ct_score"`
	CTKills            int    `json:"ct_kills"`
	CTPlayersRemaining int    `json:"ct_players_remaining"`

	T                 string `json:"t"`
	TScore            int    `json:"t_score"`
	PreviousTScore    int    `json:"previous_t_score"`
	TKills            int    `json:"t_kills"`
	TPlayersRemaining int    `json:"t_players_remaining"`

	Duration         int    `json:"duration"`
	CutDuration      int    `json:"cut_duration"`
	Winner           string `json:"winner"`
	EndReason        string `json:"end_reason"`
	WasMismatchKills bool   `json:"was_mismatch_kills"`

	mismatchKills  bool
	playing        bool
	roundEnded     bool
	lastTFragger   int
	lastCTFragger  int
	tFragRow       int
	ctFragRow      int
	previousTName  string
	previousCTName string
	previousRound  int
}

type Game struct {
	Id             string  `json:"id"`
	Version        string  `json:"version"`
	Team1          string  `json:"team_1"`
	Team2          string  `json:"team_2"`
	MapName        string  `json:"map_name"`
	RoundsNumber   int     `json:"rounds_number"`
	TickRate       float64 `json:"tick_rate"`
	TickTime       float64 `json:"tick_time"`
	MaxTicks       int     `json:"max_ticks"`
	MaxTime        float64 `json:"max_time"`
	MatchStartTick int     `json:"match_start_tick"`
	MatchEndTick   int     `json:"match_end_tick"`
	Team1Result    int     `json:"team_1_result"`
	Team2Result    int     `json:"team_2_result"`
	Winner         string  `json:"winner"`
	Rounds         []Round `json:"rounds"`
	team1          team
	team2          team
	isWinnerScreen bool
}

type team struct {
	id   int
	pos  int
	name string
}

var newOnly bool
var version = "1.1"

func main() {
	var demos []string
	for _, arg := range os.Args[1:] {
		if arg == "--new" {
			newOnly = true
		} else if strings.HasPrefix(arg, "--") {
			fmt.Printf("Prefix %s does not exist\n", arg)
		} else {
			demos = append(demos, getDemos(arg)...)
		}
	}
	if len(demos) == 0 {
		fmt.Println("There are no demos in the provided path")
		os.Exit(0)
	}
	sem := lib.NewSemaphore(6)
	count := len(demos)
	bar := pb.StartNew(count)
	for _, demo := range demos {
		sem.Add()
		go func(demo string) {
			defer sem.Done()
			processDemos(demo)
			bar.Increment()
		}(demo)
	}
	sem.Wait()
	bar.Finish()
}

func getDemos(path string) []string {
	var demos []string
	fmt.Printf("Starting to search in %s\n", path)
	if exists(path) && isFile(path) {
		if strings.HasSuffix(path, ".dem") {
			demos = append(demos, path)
			return demos
		}
		return demos
	}

	if path == "" {
		path = "."
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if !exists(path) {
		panic(fmt.Sprintf("Path (%s) provided does not exist", path))
	}
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed opening directory: %s", err)
	}
	defer file.Close()

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
	baseDemoFile := strings.Replace(demoFile, ".dem", "", 1)
	jsonFile := baseDemoFile + ".json"
	if newOnly && exists(jsonFile) {
		return
	}
	f, err := os.Open(demoFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	p := dem.NewParser(f)
	defer p.Close()
	header, err := p.ParseHeader()
	if err != nil {
		panic(err)
	}
	var game Game
	game.Version = version
	var round Round
	rounds := make([]Round, 0)
	round.playing = true
	game.MapName = header.MapName
	game.TickRate = float64(header.PlaybackTicks) / header.PlaybackTime.Seconds()
	if header.PlaybackTime > 0 && header.FrameTime() > 0 {
		game.TickTime = 1 / game.TickRate
	}
	game.MaxTicks = header.PlaybackTicks
	if header.PlaybackTime > 0 {
		game.MaxTime = header.PlaybackTime.Seconds()
	}
	if header.SignonLength == 0 {
		fmt.Printf("The demo (%s) seems to be broken\n", demoFile)
		return
	}
	game.Id = fmt.Sprintf("%s_%d%d", game.MapName, header.SignonLength, header.PlaybackTicks)

	// Register handler on kill events
	p.RegisterEventHandler(func(e events.IsWarmupPeriodChanged) {
		if game.isWinnerScreen {
			return
		}
		rounds = make([]Round, 0) // Reset the rounds
		game.team1 = team{}
		game.team2 = team{}
	})

	p.RegisterEventHandler(func(e events.AnnouncementWinPanelMatch) {
		game.isWinnerScreen = true
	})

	p.RegisterEventHandler(func(e events.RoundEndOfficial) {
		gs := p.GameState()

		// There are many cases that indicate that round was either restarted/was warmup/hasn't started yet/or round was too short..
		offset := gs.IngameTick() - int(game.TickRate)
		roundNum := gs.TeamTerrorists().Score() + gs.TeamCounterTerrorists().Score()
		if !round.playing || (!gs.IsMatchStarted() && !game.isWinnerScreen) || gs.IsWarmupPeriod() || round.StartTick == gs.IngameTick() || round.StartTick > offset {
			return
		} else if gs.TeamTerrorists().Score() == 0 && gs.TeamCounterTerrorists().Score() == 0 {
			return
		} else if roundNum < round.previousRound || roundNum > (len(rounds)+1) { // This could indicate broken round...
			return
		}
		round.playing = false
		if !game.isWinnerScreen {
			round.OfficialEndTick = gs.IngameTick()
		} else if game.MatchEndTick > 0 {
			round.OfficialEndTick = game.MatchEndTick
		} else {
			round.OfficialEndTick = game.MaxTicks
		}
		round = handleRound(gs, &game, round)
		rounds = append(rounds, round)
		round = Round{} // Restart it
	})

	p.RegisterEventHandler(func(e events.TeamSideSwitch) {
		if game.team1.id == 2 {
			game.team1.id = 3
			game.team2.id = 2
		}
		if game.team2.id == 2 {
			game.team2.id = 3
			game.team1.id = 2
		}
	})

	p.RegisterEventHandler(func(e events.FrameDone) {
		if round.mismatchKills {
			gs := p.GameState()
			round.mismatchKills = false
			round.TKills = getTKills(gs)
			round.CTKills = getCTKills(gs)
		}
	})

	p.RegisterEventHandler(func(e events.Kill) {
		gs := p.GameState()
		if gs.IngameTick() == 0 {
			return
		}
		if !round.playing || !gs.IsMatchStarted() || gs.IsWarmupPeriod() {
			return
		}

		switch e.Victim.Team {
		case common.TeamTerrorists:
			if e.Killer != nil && e.Killer.Team == common.TeamCounterTerrorists {
				round.CTKills++
				if round.lastCTFragger == e.Killer.EntityID {
					round.ctFragRow++
				} else {
					round.lastCTFragger = e.Killer.EntityID
					round.ctFragRow = 1
				}
				if round.CTKills == 5 && !round.BombPlanted {
					round.SaveReason("team_kill")
				}
			}
		case common.TeamCounterTerrorists:
			if e.Killer != nil && e.Killer.Team == common.TeamTerrorists {
				round.TKills++
				if round.lastTFragger == e.Killer.EntityID {
					round.tFragRow++
				} else {
					round.lastTFragger = e.Killer.EntityID
					round.tFragRow = 1
				}
				if round.TKills == 5 {
					round.SaveReason("team_kill")
				}
			}
		}
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		gs := p.GameState()
		if e.Winner == common.TeamSpectators {
			round = Round{}
		}
		if (e.Reason == events.RoundEndReasonCTWin && round.CTKills < 5) || (e.Reason == events.RoundEndReasonTerroristsWin && round.TKills < 5) {
			round.mismatchKills = true
			round.WasMismatchKills = true
			round.TKills = getTKills(gs)
			round.CTKills = getCTKills(gs)
		}
		round.roundEnded = true
		round.EndTick = gs.IngameTick()
		// This will be updated later on possibly otherwise it counts as officially ending
		round.OfficialEndTick = gs.IngameTick()

		if round.EndReason != "team_kill" && e.Reason != events.RoundEndReasonBombDefused && e.Reason != events.RoundEndReasonTargetBombed {
			round.SaveReason("expired")
		}
	})

	p.RegisterEventHandler(func(e events.MatchStart) {
		gs := p.GameState()
		game.MatchStartTick = gs.IngameTick()
		round.roundEnded = false
	})

	p.RegisterEventHandler(func(e events.MatchStartedChanged) {
		gs := p.GameState()
		if e.NewIsStarted && game.MatchStartTick == 0 { // If match start is not received
			game.MatchStartTick = gs.IngameTick()
			game.MatchEndTick = 0
		}
		if !e.NewIsStarted {
			game.MatchEndTick = gs.IngameTick()
		}
	})

	p.RegisterEventHandler(func(e events.RoundStart) {
		gs := p.GameState()
		if gs.IsWarmupPeriod() || !gs.IsMatchStarted() || gs.IngameTick() == round.EndTick {
			return
		}
		round.StartTick = gs.IngameTick()
		round.roundEnded = false
		round.playing = true
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		gs := p.GameState()
		if gs.IsWarmupPeriod() || !gs.IsMatchStarted() {
			return
		}
		if gs.IngameTick() == round.EndTick || (round.UnfreezeTick > 0 && round.UnfreezeTick > round.StartTick) {
			return
		}

		if round.StartTick == 0 {
			round.StartTick = gs.IngameTick()
		}
		round.playing = true
		round.roundEnded = false
		round.UnfreezeTick = gs.IngameTick()
		round.previousRound = gs.TotalRoundsPlayed()
		round.PreviousTScore = gs.TeamTerrorists().Score()
		round.previousTName = gs.TeamTerrorists().ClanName()
		round.PreviousCTScore = gs.TeamCounterTerrorists().Score()
		round.previousCTName = gs.TeamCounterTerrorists().ClanName()
		round.CTKills = 0
		round.TKills = 0
		round.EndReason = ""
	})

	p.RegisterEventHandler(func(e events.BombPlanted) {
		gs := p.GameState()
		if round.playing && gs.IsMatchStarted() {
			round.BombExploded = false // Reset them as false positive
			round.BombDefused = false  // Reset them as false positive
			round.BombPlanted = true
		}
	})

	p.RegisterEventHandler(func(e events.BombDefused) {
		gs := p.GameState()
		if round.playing && gs.IsMatchStarted() {
			round.SaveReason("bomb_defused")
			round.BombDefused = true
			round.BombDefusedAt = gs.IngameTick()
		}
	})

	p.RegisterEventHandler(func(e events.BombExplode) {
		gs := p.GameState()
		if round.playing && gs.IsMatchStarted() {
			round.SaveReason("bomb_exploded")
			round.BombExploded = true
			round.BombExplodedAt = gs.IngameTick()
		}
	})

	// Parse to end
	err = p.ParseToEnd()
	gs := p.GameState()
	if round.playing { // Just in case the "ScoreUpdated" is not reported, we add another round at the end (last)
		round = handleRound(gs, &game, round)
		rounds = append(rounds, round)
		round = Round{} // Restart it
	}

	if err != nil {
		panic(err)
	}
	game.Rounds = rounds
	game.RoundsNumber = len(rounds)
	jsonOutput, _ := json.MarshalIndent(game, "", "  ")
	err = ioutil.WriteFile(jsonFile, jsonOutput, 0644)
}

func gameUpdates(game *Game, gs dem.GameState) {
	setTeams(game, gs)
	team1, team2 := getTeamByPos(*game, gs)
	game.Team1Result = team1.Score()
	game.Team2Result = team2.Score()

	if game.Team1Result > game.Team2Result {
		game.Winner = game.Team1
	}
	if game.Team2Result > game.Team1Result {
		game.Winner = game.Team2
	}
}

func setTeams(game *Game, gs dem.GameState) {
	if game.team1.id == 0 && gs.TeamCounterTerrorists().ClanName() != "" {
		game.team1 = team{id: gs.TeamCounterTerrorists().ID(), name: gs.TeamCounterTerrorists().ClanName()}
		game.Team1 = gs.TeamCounterTerrorists().ClanName()
	}
	if game.team2.id == 0 && gs.TeamTerrorists().ClanName() != "" {
		game.team2 = team{id: gs.TeamTerrorists().ID(), name: gs.TeamTerrorists().ClanName()}
		game.Team2 = gs.TeamTerrorists().ClanName()
	}
}

func handleRound(gs dem.GameState, game *Game, round Round) Round {
	gameUpdates(game, gs)

	tTeam, ctTeam := getTeamBySide(*game, gs)
	round.T = tTeam.name
	round.CT = ctTeam.name

	tScore := gs.TeamTerrorists().Score()
	ctScore := gs.TeamCounterTerrorists().Score()
	if tScore > round.PreviousTScore {
		round.Winner = round.T
	}
	if ctScore > round.PreviousCTScore {
		round.Winner = round.CT
	}

	round.RoundNumber = ctScore + tScore
	round.CTScore = ctScore
	round.TScore = tScore
	if round.EndTick == 0 {
		round.EndTick = gs.IngameTick()
	}
	round.playing = false
	round.Duration = int(math.Round(float64(round.EndTick-round.StartTick) * game.TickTime))
	round.CutDuration = int(math.Round(float64(round.EndTick-round.UnfreezeTick) * game.TickTime))
	round.Ace = round.tFragRow == 5 || round.ctFragRow == 5
	round.CTPlayersRemaining = 5 - getTKills(gs)
	round.TPlayersRemaining = 5 - getCTKills(gs)
	if round.Ace {
		if round.tFragRow == 5 && round.lastTFragger > 0 {
			round.AceBy = gs.Participants().FindByHandle(round.lastTFragger).Name
		}
		if round.ctFragRow == 5 && round.lastCTFragger > 0 {
			round.AceBy = gs.Participants().FindByHandle(round.lastCTFragger).Name
		}
	}
	return round
}

func getCTKills(state dem.GameState) int {
	kills := 5
	for _, player := range state.Participants().TeamMembers(common.TeamTerrorists) {
		health := player.Health()
		if health > 0 {
			kills--
		}
	}
	return kills
}

func getTKills(state dem.GameState) int {
	kills := 5
	for _, player := range state.Participants().TeamMembers(common.TeamCounterTerrorists) {
		health := player.Health()
		if health > 0 {
			kills--
		}
	}
	return kills
}

func getTeamByPos(game Game, gs dem.GameState) (*common.TeamState, *common.TeamState) {
	if gs.TeamTerrorists().ID() == game.team1.id {
		return gs.TeamTerrorists(), gs.TeamCounterTerrorists()
	}
	return gs.TeamCounterTerrorists(), gs.TeamTerrorists()
}

func getTeamBySide(game Game, gs dem.GameState) (team, team) {
	if gs.TeamTerrorists().ID() == game.team1.id {
		return game.team1, game.team2
	}
	return game.team2, game.team1
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		panic(err)
		return false
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		return false
	case mode.IsRegular():
		return true
	}
	return false
}

func (r *Round) SaveReason(reason string) {
	if r.EndReason == "" {
		r.EndReason = reason
	}
}

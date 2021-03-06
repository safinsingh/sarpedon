package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	dbName      = "sarpedon"
	dbUri       = "mongodb://localhost:27017"
	mongoClient *mongo.Client
	mongoCtx    context.Context
	timeConn    time.Time
)

type scoreEntry struct {
	Time           time.Time     `json:"time,omitempty"`
	Team           teamData      `json:"team,omitempty"`
	Image          imageData     `json:"image,omitempty"`
	Vulns          vulnWrapper   `json:"vulns,omitempty"`
	Debug          string        `json:"debug,omitempty"`
	Points         int           `json:"points,omitempty"`
	Penalties      int           `json:"penalties,omitempty"`
	PlayTime       time.Duration `json:"playtime,omitempty"`
	PlayTimeStr    string        `json:"playtimestr,omitempty"`
	ElapsedTime    time.Duration `json:"elapsedtime,omitempty"`
	ElapsedTimeStr string        `json:"playtimestr,omitempty"`
}

type vulnWrapper struct {
	VulnsScored int        `json:"vulnsscored,omitempty"`
	VulnsTotal  int        `json:"vulnstotal,omitempty"`
	VulnItems   []vulnItem `json:"vulnitems,omitempty"`
}

type vulnItem struct {
	VulnText   string `json:"vulntext,omitempty"`
	VulnPoints int    `json:"vulnpoints,omitempty"`
}

type adminData struct {
	Username, Password string
}

type imageData struct {
	Name, Color string
	Records     []scoreEntry
	Index       int
}

type imageShell struct {
	Waiting     bool
	Active      bool
	StdinRead   *io.PipeReader
	StdinWrite  *io.PipeWriter
	StdoutRead  *io.PipeReader
	StdoutWrite *io.PipeWriter
}

type teamData struct {
	Id, Alias, Email  string
	ImageCount, Score int
	Time              string
}

type announcement struct {
	Time  time.Time
	Title string
	Body  string
}

func initDatabase() {
	refresh := false

	if timeConn.IsZero() {
		refresh = true
	} else {
		err := mongoClient.Ping(context.TODO(), nil)
		if err != nil {
			refresh = true
			mongoClient.Disconnect(mongoCtx)
		}
	}
	timeConn = time.Now()

	if refresh {
		fmt.Println("Refreshing mongodb connection...")
		client, err := mongo.NewClient(options.Client().ApplyURI(dbUri))
		if err != nil {
			log.Fatal(err)
		} else {
			mongoClient = client
		}
		ctx := context.TODO()
		err = client.Connect(ctx)
		if err != nil {
			log.Fatal(err)
		} else {
			mongoCtx = ctx
		}
	}
}

func getAll(teamName, imageName string) []scoreEntry {
	scores := []scoreEntry{}
	coll := mongoClient.Database(dbName).Collection("results")
	teamObj := getTeam(teamName)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{"time", 1}})

	var cursor *mongo.Cursor
	var err error

	if imageName != "" {
		fmt.Println("image specificed, searching for all records ")
		cursor, err = coll.Find(context.TODO(), bson.D{{"team.id", teamObj.Id}, {"image", getImage(imageName)}}, findOptions)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("no imag, seaaaaarrchchhin", teamObj.Id, imageName)
		cursor, err = coll.Find(context.TODO(), bson.D{{"team.id", teamObj.Id}}, findOptions)
		if err != nil {
			panic(err)
		}
	}

	if err := cursor.All(mongoCtx, &scores); err != nil {
		panic(err)
	}

	// fmt.Println("all score results", scores)
	return scores
}

func initScoreboard() {
	initDatabase()
	coll := mongoClient.Database(dbName).Collection("scoreboard")
	err := coll.Drop(mongoCtx)
	if err != nil {
		fmt.Println("error dropping scoreboard:", err)
		os.Exit(1)
	}
	topBoard, err := getScores()
	if err != nil {
		fmt.Println("error fetching scores:", err)
		os.Exit(1)
	}
	if len(topBoard) > 0 {
		topBoardInterface := []interface{}{}
		for _, item := range topBoard {
			topBoardInterface = append(topBoardInterface, item)
		}
		_, err = coll.InsertMany(context.TODO(), topBoardInterface, nil)
		if err != nil {
			fmt.Println("error inserting scores:", err)
			os.Exit(1)
		}
	}
}

func getScores() ([]scoreEntry, error) {
	initDatabase()
	scores := []scoreEntry{}
	coll := mongoClient.Database(dbName).Collection("results")

	groupStage := bson.D{
		{"$group", bson.D{
			{"_id", bson.D{
				{"image", "$image.name"},
				{"team", "$team.id"},
			}},
			{"time", bson.D{
				{"$max", "$time"},
			}},
			{"team", bson.D{
				{"$last", "$team"},
			}},
			{"image", bson.D{
				{"$last", "$image"},
			}},
			{"points", bson.D{
				{"$last", "$points"},
			}},
			{"playtime", bson.D{
				{"$last", "$playtime"},
			}},
			{"elapsedtime", bson.D{
				{"$last", "$elapsedtime"},
			}},
			{"playtimestr", bson.D{
				{"$last", "$playtimestr"},
			}},
			{"elapsedtimestr", bson.D{
				{"$last", "$elapsedtimestr"},
			}},
			{"vulns", bson.D{
				{"$last", "$vulns"},
			}},
			{"debug", bson.D{
				{"$last", "$debug"},
			}},
		}},
	}

	projectStage := bson.D{
		{"$project", bson.D{
			{"time", "$time"},
			{"team", "$team"},
			{"image", "$image"},
			{"points", "$points"},
			{"playtime", "$playtime"},
			{"elapsedtime", "$elapsedtime"},
			{"playtimestr", "$playtimestr"},
			{"elapsedtimestr", "$elapsedtimestr"},
			{"vulns", "$vulns"},
			{"debug", "$debug"},
		}},
	}

	opts := options.Aggregate()

	cursor, err := coll.Aggregate(context.TODO(), mongo.Pipeline{groupStage, projectStage}, opts)
	if err != nil {
		return scores, err
	}

	if err = cursor.All(context.TODO(), &scores); err != nil {
		return scores, err
	}

	return scores, nil
}

func getTop() ([]scoreEntry, error) {
	initDatabase()
	scores := []scoreEntry{}
	coll := mongoClient.Database(dbName).Collection("scoreboard")

	opts := options.Find()
	cursor, err := coll.Find(context.TODO(), bson.D{}, opts)
	if err != nil {
		return scores, err
	}

	if err = cursor.All(context.TODO(), &scores); err != nil {
		return scores, err
	}

	return scores, nil
}

func getCsv() string {
	teamScores, err := getTop()
	if err != nil {
		panic(err)
	}
	csvString := "Email,Alias,Team Id,Image,Score,Play Time,Elapsed Time\n"
	for _, score := range teamScores {
		csvString += score.Team.Email + ","
		csvString += score.Team.Alias + ","
		csvString += score.Team.Id + ","
		csvString += score.Image.Name + ","
		csvString += fmt.Sprintf("%d,", score.Points)
		csvString += formatTime(score.PlayTime) + ","
		csvString += formatTime(score.ElapsedTime) + "\n"
	}
	return csvString
}

func getScore(teamName, imageName string) []scoreEntry {
	scoreResults := []scoreEntry{}
	teamObj := getTeam(teamName)
	teamScores, err := getTop()
	if err != nil {
		panic(err)
	}
	if imageName != "" {
		for _, score := range teamScores {
			if score.Image.Name == imageName && score.Team.Id == teamObj.Id {
				scoreResults = append(scoreResults, score)
			}
		}
	} else {
		for _, image := range sarpConfig.Image {
			for _, score := range teamScores {
				if score.Image.Name == image.Name && score.Team.Id == teamObj.Id {
					scoreResults = append(scoreResults, score)
				}
			}
		}
	}

	return scoreResults
}

func insertScore(newEntry scoreEntry) error {
	initDatabase()
	coll := mongoClient.Database(dbName).Collection("results")
	_, err := coll.InsertOne(context.TODO(), newEntry)
	if err != nil {
		return err
	}
	return nil
}

func replaceScore(newEntry *scoreEntry) error {
	initDatabase()
	coll := mongoClient.Database(dbName).Collection("scoreboard")
	_, err := coll.DeleteOne(context.TODO(), bson.D{{"image.name", newEntry.Image.Name}, {"team.id", newEntry.Team.Id}})
	if err != nil {
		return err
	}
	_, err = coll.InsertOne(context.TODO(), newEntry)
	if err != nil {
		return err
	}
	return nil
}

func getLastScore(newEntry *scoreEntry) (scoreEntry, error) {
	initDatabase()
	score := scoreEntry{}
	coll := mongoClient.Database(dbName).Collection("scoreboard")
	err := coll.FindOne(context.TODO(), bson.D{{"image.name", newEntry.Image.Name}, {"team.id", newEntry.Team.Id}}).Decode(&score)
	if err != nil {
		fmt.Println("error finding last score:", err)
	}
	return score, err
}

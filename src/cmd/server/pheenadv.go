// Copyright 2012 The Ephenation Authors
//
// This file is part of Ephenation.
//
// Ephenation is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3.
//
// Ephenation is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Ephenation.  If not, see <http://www.gnu.org/licenses/>.
//

package main

import (
	"chunkdb"
	"ephenationdb"
	"flag"
	"fmt"
	"github.com/larspensjo/config"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"license"
	"log"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"superchunk"
	"time"
	"traffic"
)

var (
	tflag               = flag.Bool("dotest", false, "Run the test suite and then terminate.")
	procFlag            = flag.Int("p", 2, "Number of processes to use")
	ipPort              = flag.String("i", ":57862", "IP port to listen on")
	logFileName         = flag.String("log", "worldserver.log", "Log file name")
	allowTestUser       = flag.Bool("testuser", false, "Allow connection of testusers without password named 'testX', where X is a number")
	verboseFlag         = flag.Int("v", 0, "Verbose, Higher number gives more")
	cpuprofile          = flag.String("cpuprofile", "", "write cpu profile to file")
	convertChunkFiles   = flag.Bool("convertChunk", false, "Convert chunk files to new file format")
	welcomeMsgFile      = flag.String("welcome", "welcome.txt", "The file that is displayed at login")
	logOnStdout         = flag.Bool("s", false, "Send log file to standard otput")
	inhibitCreateChunks = flag.Bool("nocreate", false, "Only load modified chunks, and save no changes")
	configFileName      = flag.String("configfile", "config.ini", "General configuration file")
	createuser          = flag.String("createuser", "", "Create user from argument 'email,password,avatar'")
	bootDate            = time.Now()

	trafficStatistics = traffic.New()
	superChunkManager = superchunk.New(CnfgSuperChunkFolder)
	encryptionSalt    = ""
)

func main() {
	flag.Parse()

	if !*logOnStdout {
		logFile, _ := os.OpenFile(*logFileName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		log.SetOutput(logFile)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cnfg, err := config.ReadDefault(*configFileName)
	if err != nil {
		log.Println("Fail to find", *configFileName, err)
		return
	}
	configSection := "db"
	if cnfg.HasSection(configSection) {
		f := func(key string) string {
			value, err := cnfg.String(configSection, key)
			if err != nil {
				log.Println("Config file", *configFileName, "Failt to find key", key, err)
				return ""
			}
			return value
		}
		err = ephenationdb.SetConnection(f)
		if err != nil {
			log.Println("main: open DB:", err)
			// Continue without DB. Only test users can connect.
		}
	} else {
		log.Println("Config file", *configFileName, "missing section", configSection)
	}
	if encryptionSalt, err = cnfg.String("login", "salt"); err != nil {
		encryptionSalt = "" // Effectively no salt
	}

	if *createuser != "" {
		CreateUser(*createuser)
		return
	}

	if *convertChunkFiles {
		ConvertFiles()
		return
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile() // Also done from special command /shutdown
	}
	if *tflag {
		DoTest()
		return
	}
	log.Printf("Pheenadv world server\n")
	if *verboseFlag > 0 {
		log.Printf("Verbose flag set to %d\n", *verboseFlag)
	}
	if *inhibitCreateChunks {
		log.Println("No chunks will be created or saved")
	}
	runtime.GOMAXPROCS(*procFlag)
	rand.Seed(time.Now().UnixNano())
	host, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	log.Printf("Start world server on %s\n", host)
	if *allowTestUser {
		log.Printf("Testusers without password allowed\n")
	}
	err = SetupListenForClients_WLuBlWLqWLa(*ipPort)
	if err != nil {
		log.Printf("%v, server abort\n", err)
		os.Exit(1)
	}
	go ProcAutosave_RLu()
	go ProcPurgeOldChunks_WLw()
	go CatchSig()
	ManageMonsters_WLwWLuWLqWLmBlWLc() // Will not return
}

// Read all chunks, update them, and write them back again
// This function can also be used for convert from one chunk file format to another, with some tweaking.
func ConvertFiles() {
	dir, err := ioutil.ReadDir(CnfgChunkFolder)
	if err != nil {
		fmt.Printf("Failed to read . (%v)", err)
		return
	}
	var mod, unmod int
	for _, fi := range dir {
		fn := fi.Name()
		// fmt.Printf("%v ", fn)
		coords := strings.Split(fn, ",")
		if len(coords) != 3 {
			fmt.Printf("Skipping %v, bad file name for chunk\n", fn)
			continue
		}
		x, err := strconv.Atoi(coords[0])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		y, err := strconv.Atoi(coords[1])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		z, err := strconv.Atoi(coords[2])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		c := chunkdb.CC{X: int32(x), Y: int32(y), Z: int32(z)}
		ch := dBFindChunkFromFS(c)
		if ch.flag&CHF_MODIFIED != 0 {
			mod++
		} else {
			unmod++
			name := DBChunkFileName(c)
			err = os.Remove(name)
			if err != nil {
				fmt.Printf("Failed to remove unmodified file %v, err %v\n", fn, err)
			}
		}
	}
	fmt.Printf("%d Modified, %d non modified\n", mod, unmod)
}

// Helper function to create a user (license) and an avatar for that user
func CreateUser(str string) {
	args := strings.Split(str, ",")
	if len(args) != 3 && len(args) != 4 {
		fmt.Println("Usage: server -createuser=email,password,avatar[,licensekey]")
		return
	}
	var up user
	up.New_WLwWLc(args[2])
	up.Email = args[0]
	up.License, up.Password = license.Make(args[1], "")
	up.License = args[3] // Override
	c := ephenationdb.New().C("counters")
	var id struct {
		C uint32
	}
	change := mgo.Change{
		Update: bson.M{"$inc": bson.M{"c": 1}},
	}
	_, err := c.FindId("avatarId").Apply(change, &id)
	if err != nil {
		fmt.Println("Failed to update unique counter 'avatarId' in collection 'counter'", err)
		return
	}
	up.Id = id.C
	db := ephenationdb.New()
	err = db.C("avatars").Insert(&up)
	if err != nil {
		log.Println("Save", up.Name, err)
		return
	}
	fmt.Println("Created avatar number", up.Id, ":", up.Name)
}

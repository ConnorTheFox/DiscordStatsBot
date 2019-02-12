package main

import (
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
)

var userSettingsMap = make(map[string]*keepTrackOfMsg)

type waitingMsg struct {
	msgID          string
	channelID      string
	middleMsg      string
	currentTicker  *time.Ticker
	currentSession *discordgo.Session
}

type keepTrackOfMsg struct {
	id      string
	command string
	timer   *time.Timer
}

func (waiting *waitingMsg) send(channelID string) {
	var currentClock int
	clocks := [11]int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	msg, _ := waiting.currentSession.ChannelMessageSend(channelID, ":clock1: "+waiting.middleMsg+" :clock1:")
	waiting.msgID = msg.ID
	waiting.channelID = channelID
	ticker := time.NewTicker(time.Millisecond * 1000)
	waiting.currentTicker = ticker
	go func() {
		for range ticker.C {
			currentClockStr := " :clock" + strconv.Itoa(clocks[currentClock]) + ": "
			waiting.currentSession.ChannelMessageEdit(channelID, msg.ID, currentClockStr+waiting.middleMsg+currentClockStr)
			currentClock++
			if currentClock == 10 {
				currentClock = 0
			}
		}
	}()
}

func (waiting *waitingMsg) delete() {
	waiting.currentSession.ChannelMessageDelete(waiting.channelID, waiting.msgID)
	waiting.currentTicker.Stop()
}

func handleErrorInCommand(session *discordgo.Session, channelID string, err error, waitingMsg *waitingMsg) {
	session.ChannelMessageSend(channelID, "Sorry an error occured :( "+err.Error())
	ownerID := os.Getenv("OWNERID")
	ownerDM, _ := session.UserChannelCreate(ownerID)
	session.ChannelMessageSend(ownerDM.ID, err.Error())
	waitingMsg.delete()
	fmt.Printf("%+v\n", err)
}

func removeDiscordUser(userID, deletedGuildID string) {
	user := discordUsers[userID]
	otherGuilds := user.otherGuilds
	user.mu.Lock()
	defer user.mu.Unlock()

	if user.mainGuild == deletedGuildID { //If main guild is deleted
		if len(otherGuilds) == 0 { //No other guilds left
			if user.isPlaying == true {
				user.save()
				updateOrSave(user.mainGuild, user)
			}
			delete(discordUsers, user.mainGuild)
			return
		}
		for _, item := range otherGuilds {
			user.mainGuild = item
			break
		}
	}
	updateOrSave(deletedGuildID, user)
	delete(otherGuilds, deletedGuildID)
}

func addDiscordUser(newUserID, newGuildID string, isBot bool) {
	if isBot == false {
		if _, ok := discordUsers[newUserID]; ok == false {
			itemToInsert := setting{
				ID:              newUserID,
				GraphType:       "bar",
				MentionForStats: true,
			}
			db.insert("settings", itemToInsert)

			discordUsers[newUserID] = &discordUser{
				userID:      newUserID,
				mainGuild:   newGuildID,
				isPlaying:   false,
				otherGuilds: make(map[string]string),
			}
		} else if _, ok := discordUsers[newUserID].otherGuilds[newGuildID]; ok == false {
			discordUsers[newUserID].otherGuilds[newGuildID] = newGuildID
		}
	}
}

func addDiscordGuild(guildInfo *discordgo.Guild) {
	var presenceMap = make(map[string]*discordgo.Presence)
	for _, presence := range guildInfo.Presences {
		userID := presence.User.ID
		if _, ok := presenceMap[userID]; ok != true {
			presenceMap[userID] = presence
		}
	}
	for _, member := range guildInfo.Members {
		if member.User.Bot == false {
			if db.itemExists("settings", bson.M{"id": member.User.ID}) == false {
				itemToInsert := setting{
					ID:              member.User.ID,
					GraphType:       "bar",
					MentionForStats: true,
				}
				db.insert("settings", itemToInsert)
			}
			userID := member.User.ID
			presence := presenceMap[userID]
			if _, ok := discordUsers[userID]; ok != true {
				var currentGame string
				var isPlaying bool
				var startedPlaying time.Time
				if presence.Game != nil {
					currentGame = presence.Game.Name
					isPlaying = true
					startedPlaying = time.Now()
				}
				discordUsers[userID] = &discordUser{
					userID:         userID,
					mainGuild:      guildInfo.ID,
					currentGame:    currentGame,
					startedPlaying: startedPlaying,
					isPlaying:      isPlaying,
					otherGuilds:    make(map[string]string),
				}
			} else if ok := discordUsers[userID].otherGuilds[guildInfo.ID]; ok == "" && guildInfo.ID != discordUsers[userID].mainGuild {
				discordUsers[userID].otherGuilds[guildInfo.ID] = guildInfo.ID
			}
		}
	}
}

func processUserImg(userID, username string, avatar *image.Image) (*discordgo.MessageSend, error) {
	var userStats []stat
	db.findAll("gamestats", bson.M{"id": userID}, &userStats)
	totalHours := db.countHours(bson.M{"id": userID})
	totalGames := db.countGames(bson.M{"id": userID})
	imgReader, err := createImage(avatar, fmt.Sprint(totalHours), strconv.Itoa(totalGames), username, "bar", userID)
	if err != nil {
		return nil, err
	}
	discordMessageSend := &discordgo.MessageSend{
		Content: "Here are your stats " + username + "!",
		Files: []*discordgo.File{
			&discordgo.File{
				Name:        userID + ".png",
				ContentType: "image/png",
				Reader:      imgReader,
			},
		},
	}
	return discordMessageSend, nil
}

func processBotImg(user *discordgo.User, session *discordgo.Session) (*discordgo.MessageSend, error) {
	avatar, err := loadDiscordAvatar(user.AvatarURL("512"))
	if err != nil {
		return nil, errors.Wrap(err, "Loading bot avatar")
	}
	var botStats []stat
	var botGames []icon
	db.findAll("gamestats", bson.M{}, &botStats)
	db.findAll("gameicons", bson.M{}, &botGames)
	totalStats := strconv.Itoa(len(botStats))
	totalGames := strconv.Itoa(len(botGames))
	totalImgGen, err := ioutil.ReadFile(path.Join(dataDir, "botImg.txt"))
	if err != nil {
		return nil, errors.Wrap(err, "Reading bot file")
	}
	totalServers := strconv.Itoa(len(session.State.Guilds))
	totalUsers := strconv.Itoa(len(discordUsers))
	imgReader, err := createBotImage(avatar, user.Username, totalStats, totalGames, string(totalImgGen), totalServers, totalUsers)
	if err != nil {
		return nil, errors.Wrap(err, "Creating bot img")
	}
	discordMessageSend := &discordgo.MessageSend{
		Content: "Here are my stats!",
		Files: []*discordgo.File{
			&discordgo.File{
				Name:        user.ID + ".png",
				ContentType: "image/png",
				Reader:      imgReader,
			},
		},
	}
	return discordMessageSend, nil
}

func createMainMenu(lengthIgnoredStats, lengthUnignoredStats, graphType string, mentionSetting bool, username string) *discordgo.MessageEmbed {
	var mentionSettingStr = "disabled"
	if mentionSetting == true {
		mentionSettingStr = "enabled"
	}
	return &discordgo.MessageEmbed{
		Fields: []*discordgo.MessageEmbedField{
			&discordgo.MessageEmbedField{
				Name:  "**" + username + " Settings**",
				Value: "Below are the options that you can change, if you want to change an option send me a message with the setting you want to change.",
			},
			&discordgo.MessageEmbedField{
				Name:  "graph (" + graphType + ")",
				Value: "This setting let's you change your graph type.",
			},
			&discordgo.MessageEmbedField{
				Name:  "hide (" + lengthUnignoredStats + " currently showing)",
				Value: "This setting let's you hide games from your stats.",
			},
			&discordgo.MessageEmbedField{
				Name:  "show (" + lengthIgnoredStats + " currently hidden)",
				Value: "This setting let's you show games from your stats that are ignored.",
			},
			&discordgo.MessageEmbedField{
				Name:  "mention (" + mentionSettingStr + ")",
				Value: "This lets you disable other people mentioning you to get your stats.",
			},
			&discordgo.MessageEmbedField{
				Name:  "show hidden",
				Value: "This shows you your hidden games.",
			},
		},
	}
}

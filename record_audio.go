package main

import (
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	ctx = context.Background()
	dgVoice *discordgo.Session
	connection *discordgo.VoiceConnection
	fileLocation uint32
	)




func main() {
	LoadEnv()
	Token := GetEnvWithKey("Token")
	var err error
	dgVoice, err = discordgo.New("Bot " + Token)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return
	}
	defer dgVoice.Close()
	dgVoice.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildVoiceStates)
	err = dgVoice.Open()
	if err != nil {
		log.Println("error opening connection:", err)
		return
	}
	handleMessages()
}

func GetEnvWithKey(key string) string {
	return os.Getenv(key)
}

func LoadEnv() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file")
		os.Exit(1)
	}
}



func createPitonRTPPacket(p *discordgo.Packet) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version: 2,
			// Taken from Discord voice docs
			PayloadType:    0x78,
			SequenceNumber: p.Sequence,
			Timestamp:      p.Timestamp,
			SSRC:           p.SSRC,
		},
		Payload: p.Opus,
	}
}


func handleVoice(c chan *discordgo.Packet, channel string) {
	files := make(map[uint32]media.Writer)
	for p := range c {
		file, ok := files[p.SSRC]
		if !ok {
			fileLocation= p.SSRC
			var err error
			file, err = oggwriter.New(fmt.Sprintf("%d.ogg", p.SSRC), 48000, 2)
			if err != nil {
				log.Printf("failed to create file %d.ogg, giving up on recording: %v\n", p.SSRC, err)
				return
			}
			files[p.SSRC] = file
		}
		rtp := createPitonRTPPacket(p)
		err := file.WriteRTP(rtp)
		if err != nil {
			log.Printf("failed to write to file %d.ogg, giving up on recording: %v\n", p.SSRC, err)
		}
	}

	for _, f := range files {
		f.Close()
		log.Println(fileLocation)
		log.Println("Closed file")
	}
}

func handleConfig(status bool, channelName string) {


	if status {
		GuildID := GetEnvWithKey("GuildID")
		ChannelID := GetEnvWithKey("ChannelID")
		channels, _ := dgVoice.GuildChannels(GuildID)
		for _, c := range channels {
			if c.Name == channelName {
				ChannelID = c.ID
			}
		}

		v, err := dgVoice.ChannelVoiceJoin(GuildID, ChannelID, true, false)
		if err != nil {
			log.Println("failed to join voice channel:", err)
			return
		}
		connection=v
		log.Println("JOINING CHANNEL")
		handleVoice(v.OpusRecv,channelName)
	} else {
		log.Println("LEAVING CHANNEL")
		close(connection.OpusRecv)
		connection.Close()
		connection.Disconnect()
		AddtoS3(fmt.Sprint(fileLocation)+".ogg")
	}
}

func handleMessages() {
	Token := GetEnvWithKey("Token")
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return
	}
	dg.AddHandler(messageCreate)
	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildMessages)
	err = dg.Open()
	if err != nil {
		log.Println("error opening connection,", err)
		return
	}
	log.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	stri := strings.Split(m.Content, " ")
	if len(stri) >= 1 {
		if stri[1] == "join" {
			stri = strings.Split(m.Content, "join ")
			s.ChannelMessageSend(m.ChannelID, "Got it! Joining Channel: "+stri[1])
			handleConfig(true, stri[1])
		} else if stri[1] == "leave" {
			stri = strings.Split(m.Content, "leave ")
			s.ChannelMessageSend(m.ChannelID, "Got it! Leaving Channel: "+stri[1])
			handleConfig(false, stri[1])
		}
	}

}





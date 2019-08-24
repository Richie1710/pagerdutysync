package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/Richie1710/gokronolith"
	"github.com/spf13/viper"
)

type usermapping struct {
	Usermappings []usermappinguser `mapstructure:"usermapping"`
}
type usermappinguser struct {
	Pdname   string `mapstructure:"pdname"`
	Ldapname string `mapstructure:"ldapname"`
	Cuser    string `mapstructure:"cuser"`
}

func main() {
	schedulename, err := getSchedulenameByConfig()
	if err != nil {
		log.Fatalln(err)
	}
	if schedulename == "" {
		log.Fatalln("Cannot read schedulename from config")
	}
	authtoken, err := getAuthtokenByConfig()

	if err != nil {
		log.Fatalln(err)
	}
	if authtoken == "" {
		log.Fatalln("Cannot read authtoken from config")
	}

	if err != nil {
		log.Fatalln(err)
	}
	if authtoken == "" {
		log.Fatalln("Cannot read Usermapping from config")
	}

	ldapuser, err := getLdapUsernameByConfig()
	if err != nil {
		log.Fatalln(err)
	}

	ldappassword, err := getLdapPasswordByConfig()
	if err != nil {
		log.Fatalln(err)
	}

	kronolithurl, err := getKronolithURLByConfig()
	if err != nil {
		log.Fatalln(err)
	}

	kalenderid, err := getKronolithKalenderIDByConfig()
	if err != nil {
		log.Fatalln(err)
	}

	scheduleid, err := getScheduleIDByName(schedulename, authtoken)
	if err != nil {
		log.Fatalln(err)
	}
	if scheduleid == "" {
		log.Fatalln("ScheduleID empty.")
	}
	schedule, err := getSchedule(scheduleid, authtoken)
	if err != nil {
		log.Fatalln(err)
	}
	if schedule.Name == "" {
		log.Fatalln("schedule is empty")
	}

	test, err := gokronolith.GetEntryByTime(kronolithurl, kalenderid, ldapuser, ldappassword, time.Now().Unix(), time.Now().Add(time.Hour*time.Duration(168)).Unix())
	if err != nil {
		log.Fatalln(err)
	}
	var vcards []gokronolith.Vcard
	for _, entry := range test {
		vcardstring, err := gokronolith.GetICSByEntry(kronolithurl, entry, ldapuser, ldappassword)
		if err != nil {
			log.Fatalln(err)
		}
		vcard, err := gokronolith.GetICSObjectByEntry(vcardstring)
		if err != nil {
			log.Fatalln(err)
		}
		vcards = append(vcards, vcard)
	}
	//	vcards = gokronolith.FilterEntryObjectsByTime(vcards, time.Now().Unix(), time.Now().Add(time.Hour*time.Duration(24)).Unix())
	vcards = gokronolith.FilterEntryObjectsByTime(vcards, time.Now().Unix(), time.Now().Add(time.Hour*time.Duration(168)).Unix())

	sort.Slice(vcards, func(i, j int) bool { return vcards[i].DTSTART.Before(vcards[j].DTSTART) })

	for _, card := range vcards {
		loc, err := time.LoadLocation("Europe/Berlin")
		if err != nil {
			log.Fatalln(err)
		}
		kusera := strings.Split(card.SUMMARY, "--")
		kuser := strings.TrimSpace(kusera[1])

		id, _, err := getPDUserByKronUser(kuser, authtoken)
		if err != nil {
			log.Fatalln(err)
		}

		user, err := getPDUserbyPDID(authtoken, id)
		if err != nil {
			log.Fatalln(err)
		}

		schedule, err := getSchedule(scheduleid, authtoken)
		if err != nil {
			log.Fatalln(err)
		}
		if schedule.Name == "" {
			log.Fatalln("schedule is empty")
		}
		kpdlayer, err := getCurrentPagerdutyLayer(card.DTSTART.In(loc))
		if err != nil {
			log.Fatalln(err)
		}
		j := 0
		for _, i := range schedule.ScheduleLayers {
			vergleichsstring := fmt.Sprintf("Layer %d", kpdlayer)
			if i.Name == vergleichsstring {
				break
			}
			j = j + 1
		}

		schedule.ScheduleLayers[j].Users[0].User.ID = user.ID
		schedule.ScheduleLayers[j].Users[0].User.Summary = user.Summary
		schedule.ScheduleLayers[j].Users[0].User.Self = user.Self
		schedule.ScheduleLayers[j].Users[0].User.HTMLURL = user.HTMLURL
		schedule.ScheduleLayers[j].Start = card.DTSTART.Format(time.RFC3339)
		schedule.ScheduleLayers[j].RotationVirtualStart = card.DTSTART.Format(time.RFC3339)
		log.Printf("Setting on Layer %s at %s : %s", schedule.ScheduleLayers[j].Name, card.DTSTART.Format("2006-01-02"), user.Name)
		_, err = setPagerdutySchedule(authtoken, scheduleid, *schedule)
		if err != nil {
			log.Fatalln(err)
		}

	}

}

func getPDUserbyPDID(authtoken string, userid string) (*pagerduty.User, error) {
	client := pagerduty.NewClient(authtoken)
	var getuseropts pagerduty.GetUserOptions
	user, err := client.GetUser(userid, getuseropts)
	if err != nil {
		return user, err
	}
	return user, err
}

func getPDUserByKronUser(ldapuser string, authtoken string) (string, string, error) {

	C, err := getUsermappingByConfig()
	if err != nil {
		return "", "", err
	}
	usermap := C.Usermappings
	var userm usermappinguser
	var pdusers pagerduty.User
	var userinarray bool
	var userinpagerduty bool
	userinarray = false
	userinpagerduty = false
	for _, userm = range usermap {

		if userm.Ldapname == ldapuser {
			userinarray = true
			break
		}
	}
	if userinarray == false {
		err := fmt.Errorf("User %s not found in Userconfiguration", ldapuser)
		return "", "", err
	}

	client := pagerduty.NewClient(authtoken)
	var listuserops pagerduty.ListUsersOptions
	listuser, err := client.ListUsers(listuserops)
	if err != nil {
		return "", "", err
	}
	for _, pdusers = range listuser.Users {
		if pdusers.Name == userm.Pdname {
			userinpagerduty = true
			break
		}
	}

	if userinpagerduty == false {
		err := fmt.Errorf("User %s not found in Pagerduty", userm.Pdname)
		return "", "", err
	}

	return pdusers.ID, pdusers.Name, nil
}

func setPagerdutySchedule(authtoken string, scheduleid string, schedule pagerduty.Schedule) (pagerduty.Schedule, error) {
	client := pagerduty.NewClient(authtoken)
	newschedule, err := client.UpdateSchedule(scheduleid, schedule)
	if err != nil {
		return schedule, err
	}
	return *newschedule, nil
}

func getLayerCountByTime(time time.Time) int {
	switch int(time.Weekday()) {
	case 6, 0:
		return 2
	default:
		return 3
	}
}

func getCurrentPagerdutyLayer(time time.Time) (pdlayer int, err error) {
	layercount := getLayerCountByTime(time)
	switch layercount {
	case 2:
		switch int(time.Weekday()) {
		case 6:
			hours := time.Hour()
			switch {
			case hours < 8:
				return 4, nil
			case hours >= 8 && hours < 20:
				return 5, nil
			case hours >= 20:
				return 6, nil
			}
		case 0:
			hours := time.Hour()
			switch {
			case hours < 8:
				return 6, nil
			case hours >= 8 && hours < 20:
				return 7, nil
			case hours >= 20:
				return 1, nil
			}
		}
	case 3:
		switch int(time.Weekday()) {
		case 1, 2, 3, 4:
			hours := time.Hour()
			switch {
			case hours < 8:
				return 1, nil
			case hours >= 8 && hours < 14:
				return 2, nil

			case hours >= 14 && hours < 20:
				return 3, nil
			case hours >= 20:
				return 1, nil
			}
		case 5:
			hours := time.Hour()
			switch {
			case hours < 8:
				return 1, nil
			case hours >= 8 && hours < 14:
				return 2, nil
			case hours >= 14 && hours < 20:
				return 3, nil
			case hours >= 20:
				return 4, nil
			}
		}
	}
	err = fmt.Errorf("Something went wrong time not captured %s", time.Format("2006-01-02T15:04:05"))
	return 0, err
}

func deleteduplicatedlayers(schedule *pagerduty.Schedule) *pagerduty.Schedule {
	var newlayers []pagerduty.ScheduleLayer
	for _, s := range schedule.ScheduleLayers {
		if s.ID == "PT1I4HO" || s.ID == "P8G93SY" || s.ID == "PPSX44L" || s.ID == "PWB65WB" || s.ID == "PANU691" || s.ID == "PDJF6LR" {
			continue
		}
		newlayers = append(newlayers, s)
	}
	schedule.ScheduleLayers = newlayers
	return schedule
}

func getSchedulenameByConfig() (string, error) {
	var schedulename string

	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	schedulename = viper.GetString("SCHEDULENAME")
	return schedulename, nil
}

func getAuthtokenByConfig() (string, error) {
	var schedulename string
	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	schedulename = viper.GetString("AUTHTOKEN")
	return schedulename, nil
}

func getLdapUsernameByConfig() (string, error) {
	var ldapusername string
	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	ldapusername = viper.GetString("LDAPUSERNAME")
	return ldapusername, nil
}

func getLdapPasswordByConfig() (string, error) {
	var ldappassword string
	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	ldappassword = viper.GetString("LDAPPASSWORD")
	return ldappassword, nil
}

func getKronolithURLByConfig() (string, error) {
	var kronolithurl string
	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	kronolithurl = viper.GetString("KRONOLITHURL")
	return kronolithurl, nil
}

func getKronolithKalenderIDByConfig() (string, error) {
	var kronolithkalenderid string
	viper.SetConfigName("b1pagerdutysync")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return "", err
	}
	kronolithkalenderid = viper.GetString("KRONOLITHKALENDERID")
	return kronolithkalenderid, nil
}

func getUsermappingByConfig() (usermapping, error) {
	var C usermapping
	viper.SetConfigName("usermapping")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/b1pagerdutysync")
	err := viper.ReadInConfig()
	if err != nil {
		return C, err
	}
	err2 := viper.Unmarshal(&C)
	if err2 != nil {
		return C, err
	}
	return C, nil
}

func getScheduleIDByName(schedulename string, authtoken string) (string, error) {
	var scheduleopts pagerduty.ListSchedulesOptions

	client := pagerduty.NewClient(authtoken)
	schedules, err := client.ListSchedules(scheduleopts)
	if err != nil {
		return "", err
	}
	for _, schedule := range schedules.Schedules {
		if schedule.Name == schedulename {
			return schedule.ID, nil
		}
	}
	err = fmt.Errorf("Could not found a Schedule with Name %s", schedulename)
	return "", err
}

func getSchedule(scheduleid string, authtoken string) (*pagerduty.Schedule, error) {
	client := pagerduty.NewClient(authtoken)
	var getscheduleopts pagerduty.GetScheduleOptions
	schedule, err := client.GetSchedule(scheduleid, getscheduleopts)
	if err != nil {
		return nil, err
	}
	return schedule, err

}

func getScheduleLayerBySchedule(schedule *pagerduty.Schedule) []pagerduty.ScheduleLayer {
	return schedule.ScheduleLayers
}

func getScheduleUsers(scheduleid string, authtoken string) ([]pagerduty.APIObject, error) {
	client := pagerduty.NewClient(authtoken)
	var getscheduleopts pagerduty.GetScheduleOptions
	schedule, err := client.GetSchedule(scheduleid, getscheduleopts)
	if err != nil {
		return nil, err
	}
	return schedule.Users, err

}

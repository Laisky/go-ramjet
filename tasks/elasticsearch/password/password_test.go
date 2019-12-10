package password_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/tasks/elasticsearch/password"
	utils "github.com/Laisky/go-utils"
)

func TestGeneratePasswdByDate(t *testing.T) {
	now, _ := time.Parse("20060102 15", "20060101 12")
	secret := "fjfjkejflewjfjewfiwefijweifj"
	passwd1 := password.GeneratePasswdByDate(now, secret) // origin
	t.Logf("got pw1: %v", passwd1)
	passwd2 := password.GeneratePasswdByDate(now, secret+"1") // secret change
	t.Logf("got pw2: %v", passwd2)
	passwd3 := password.GeneratePasswdByDate(now.Add(24*40*time.Hour), secret) // new month
	t.Logf("got pw3: %v", passwd3)
	fmt.Println("now", now)
	passwd4 := password.GeneratePasswdByDate(now.Add(-24*time.Hour), secret) // new month
	t.Logf("got pw4: %v", passwd4)
	passwd5 := password.GeneratePasswdByDate(now.Add(40*time.Hour), secret) // same month
	t.Logf("got pw5: %v", passwd5)

	if passwd2 == passwd1 {
		t.Error("pw2 should not equal to pw1")
	}
	if passwd3 == passwd1 {
		t.Error("pw3 should not equal to pw1")
	}
	if passwd4 == passwd1 {
		t.Error("pw4 should not equal to pw1")
	}
	if passwd5 != passwd1 {
		t.Error("pw5 should equal to pw1")
	}

	// t.Error()
}

func init() {
	if err := utils.Logger.ChangeLevel("debug"); err != nil {
		utils.Logger.Panic("change log level", zap.Error(err))
	}
}

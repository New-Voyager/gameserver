package gamescript

import (
	"testing"
)

func TestReadPlayerConfig(t *testing.T) {
	p, err := ReadPlayersConfig("test_scripts/common/players.yaml")
	if err != nil {
		t.Fatalf("ReadPlayersConfig returned error [%s]", err)
	}
	if p == nil {
		t.Fatal("ReadPlayersConfig returned nil data")
	}
	numPlayers := len(p.Players)
	expectedNumPlayers := 15
	if numPlayers != expectedNumPlayers {
		t.Fatalf("Number of players = %d; expected %d", len(p.Players), expectedNumPlayers)
	}

	var expectedPlayers = []Player{
		{
			Name:     "yong",
			DeviceID: "c2dc2c3d-13da-46cc-8c66-caa0c77459de",
			Email:    "yong@gmail.com",
			Password: "340k7n0p",
		},
		{
			Name:     "brian",
			DeviceID: "4b93e2be-7992-45c3-a2dd-593c2f708cb7",
			Email:    "brian@hotmail.com",
			Password: "0x44m89w",
		},
	}

	for i, expectedPlayer := range expectedPlayers {
		if p.Players[i].Name != expectedPlayer.Name {
			t.Errorf("Player %d Name = %s; expected %s", i, p.Players[i].Name, expectedPlayer.Name)
		}
		if p.Players[i].DeviceID != expectedPlayer.DeviceID {
			t.Errorf("Player %d DeviceID = %s; expected %s", i, p.Players[i].DeviceID, expectedPlayer.DeviceID)
		}
		if p.Players[i].Email != expectedPlayer.Email {
			t.Errorf("Player %d Email = %s; expected %s", i, p.Players[i].Email, expectedPlayer.Email)
		}
		if p.Players[i].Password != expectedPlayer.Password {
			t.Errorf("Player %d Password = %s; expected %s", i, p.Players[i].Password, expectedPlayer.Password)
		}
	}
}

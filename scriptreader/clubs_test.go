package scriptreader

import (
	"testing"
)

func TestReadClubConfig(t *testing.T) {
	c, err := ReadClubsConfig("test_scripts/common/clubs.yaml")
	if err != nil {
		t.Fatalf("ReadClubConfig returned error [%s]", err)
	}
	if c == nil {
		t.Fatal("ReadClubConfig returned nil data")
	}
	numClubs := len(c.Clubs)
	expectedNumClubs := 2
	if numClubs != expectedNumClubs {
		t.Fatalf("Number of clubs = %d; expected %d", len(c.Clubs), expectedNumClubs)
	}

	var expectedClubs = []Club{
		{
			Name:        "Manchester Club",
			Description: "Premier gaming experience in New England",
			Owner:       "brian",
			Members:     []string{"yong", "brian", "tom", "jim", "rob", "john", "michael", "bill", "david", "rich", "josh", "chris"},
		},
		{
			Name:        "Bad Robots",
			Description: "No humans allowed invite only",
			Owner:       "yong",
			Members:     []string{"yong", "brian", "tom", "jim", "rob", "john", "michael", "bill", "david", "rich", "josh", "chris"},
		},
	}

	for i, expectedClub := range expectedClubs {
		if c.Clubs[i].Name != expectedClub.Name {
			t.Errorf("Club %d Name = %s; expected %s", i, c.Clubs[i].Name, expectedClub.Name)
		}
		if c.Clubs[i].Description != expectedClub.Description {
			t.Errorf("Club %d Description = %s; expected %s", i, c.Clubs[i].Description, expectedClub.Description)
		}
		if c.Clubs[i].Owner != expectedClub.Owner {
			t.Errorf("Club %d Owner = %s; expected %s", i, c.Clubs[i].Owner, expectedClub.Owner)
		}

		expectedNumMembers := len(expectedClub.Members)
		actualNumMembers := len(c.Clubs[i].Members)
		if actualNumMembers != expectedNumMembers {
			t.Errorf("Number of club memebers = %d; expected %d", actualNumMembers, expectedNumMembers)
		}
		for idxMember := 0; idxMember < expectedNumMembers; idxMember++ {
			if c.Clubs[i].Members[idxMember] != expectedClub.Members[idxMember] {
				t.Errorf("Club %d Members = %v; expected %v", i, c.Clubs[i].Members, expectedClub.Members)
				break
			}
		}
	}
}

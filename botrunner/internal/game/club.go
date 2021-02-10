package game

// Enum value maps for ClubMemberStatus.
var (
	ClubMemberStatus_name = map[int32]string{
		0: "UNKNOWN",
		1: "INVITED",
		2: "PENDING",
		3: "DENIED",
		4: "ACTIVE",
		5: "LEFT",
		6: "KICKEDOUT",
	}
	ClubMemberStatus_value = map[string]int32{
		"UNKNOWN":   0,
		"INVITED":   1,
		"PENDING":   2,
		"DENIED":    3,
		"ACTIVE":    4,
		"LEFT":      5,
		"KICKEDOUT": 6,
	}
)

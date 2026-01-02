package profile

// UserProfile represents the complete dossier of a user.
type UserProfile struct {
	UserID         string         `json:"user_id"`
	Identity       Identity       `json:"identity"`
	CognitiveModel CognitiveModel `json:"cognitive_model"`
	Attributes     []Attribute    `json:"attributes"`
	Stats          UserStats      `json:"stats"`
	Dossier        Dossier        `json:"dossier"`
	Topics         []Topic        `json:"topics"`
	Traits         Psychometrics  `json:"traits"`
}

type Identity struct {
	Username  string   `json:"username"`
	AvatarURL string   `json:"avatar_url"`
	FirstSeen string   `json:"first_seen"`
	Badges    []string `json:"badges"`
	Status    string   `json:"status"` // online, idle, etc.
}

type CognitiveModel struct {
	TechnicalLevel     float64 `json:"technical_level"` // 1-10
	CommunicationStyle string  `json:"communication_style"`
	PatienceLevel      string  `json:"patience_level"`
	Vibe               string  `json:"vibe"`
}

type Attribute struct {
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

type UserStats struct {
	TotalMessages  int    `json:"total_messages"`
	LastActive     string `json:"last_active"`
	TokensConsumed int64  `json:"tokens_consumed"`
}

type Dossier struct {
	Identity IdentityDetails    `json:"identity"`
	Career   CareerDetails      `json:"career"`
	Personal PersonalDetails    `json:"personal"`
	Social   []SocialConnection `json:"social"`
}

type IdentityDetails struct {
	FullName     string `json:"fullName"`
	AgeRange     string `json:"ageRange"`
	Location     string `json:"location"`
	Gender       string `json:"gender"`
	Sexuality    string `json:"sexuality"`
	Relationship string `json:"relationship"`
}

type CareerDetails struct {
	JobTitle string   `json:"jobTitle"`
	Company  string   `json:"company"`
	Skills   []string `json:"skills"`
}

type PersonalDetails struct {
	Hobbies []string `json:"hobbies"`
	Habits  []string `json:"habits"`
	Vices   []string `json:"vices"`
	Virtues []string `json:"virtues"`
}

type SocialConnection struct {
	Name     string `json:"name"`
	Relation string `json:"relation"`
	Trust    string `json:"trust"`
}

type Topic struct {
	Name string `json:"name"`
	Val  int    `json:"val"`
}

type Psychometrics struct {
	Openness          int `json:"openness"`
	Conscientiousness int `json:"conscientiousness"`
	Extraversion      int `json:"extraversion"`
	Agreeableness     int `json:"agreeableness"`
	Neuroticism       int `json:"neuroticism"`
}

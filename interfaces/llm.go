package interfaces

type Persona struct {
	Comment1    string      `json:"_comment"`
	Identity    Identity    `json:"identity"`
	Comment2    string      `json:"_comment_2"`
	Functions   Functions   `json:"functions"`
	Comment3    string      `json:"_comment_3"`
	Personality Personality `json:"personality"`
	Comment4    string      `json:"_comment_4"`
	Preferences Preferences `json:"preferences"`
}

type Identity struct {
	Name        string   `json:"name"`
	Alias       []string `json:"alias"`
	Pronouns    string   `json:"pronouns"`
	PersonaType string   `json:"persona_type"`
	OriginStory string   `json:"origin_story"`
}

type Functions struct {
	InteractionRules struct {
		Initiative     string `json:"initiative"`
		ProblemSolving string `json:"problem_solving"`
	} `json:"interaction_rules"`
}

type Personality struct {
	CoreTraits struct {
		Passion       float64 `json:"passion"`
		Humour        float64 `json:"humour"`
		Sarcasm       float64 `json:"sarcasm"`
		Empathy       float64 `json:"empathy"`
		Aggression    float64 `json:"aggression"`
		Openness      float64 `json:"openness"`
		Extraversion  float64 `json:"extraversion"`
		Agreeableness float64 `json:"agreeableness"`
	} `json:"core_traits"`
	CommunicationStyle struct {
		Verbosity struct {
			Default string `json:"default"`
		} `json:"verbosity"`
		Formality string   `json:"formality"`
		Tone      []string `json:"tone"`
		Emojis    string   `json:"emojis"`
	} `json:"communication_style"`
}

type Preferences struct {
	Content struct {
		DefaultLanguage string `json:"default_language"`
	} `json:"content"`
	Formatting struct {
		CodeBlockStyle struct {
			LineNumbers bool `json:"line_numbers"`
		} `json:"code_block_style"`
	} `json:"formatting"`
}

package scenario

// People are the first names demo data draws from (shared by seed and Generate).
func People() []string {
	return []string{"ada", "grace", "linus", "margaret", "alan", "barbara", "dennis", "ken", "radia", "tim"}
}

// Topics are channel names demo data draws from.
func Topics() []string {
	return []string{
		"random", "engineering", "design", "product", "support", "sales", "marketing", "ops", "security", "data",
		"frontend", "backend", "mobile", "infra", "qa", "docs", "hiring", "finance", "legal", "people",
		"announcements", "standup", "retro", "incidents", "oncall", "releases", "roadmap", "research", "ideas", "water-cooler",
		"music", "books", "games", "food", "travel", "fitness", "pets", "memes", "help", "feedback",
		"ci", "deploys", "alerts", "metrics", "analytics", "partners", "customers", "onboarding", "training",
	}
}

// Lines are message texts demo data draws from.
func Lines() []string {
	return []string{
		"morning all", "anyone seen the build?", "shipping this today", "can someone review my PR?",
		"lunch at noon?", "looks good to me", "nice work!", "I'll take a look", "blocked on the API change",
		"standup in 5", "pushed a fix", "thanks!", "ok let's sync later",
	}
}

// Emoji are reaction names demo data draws from.
func Emoji() []string {
	return []string{"thumbsup", "eyes", "tada", "heart", "rocket", "white_check_mark"}
}

// BotName is the demo workspace bot's user name.
const BotName = "localslack-bot"

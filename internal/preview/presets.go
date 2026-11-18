package preview

type Preset struct {
	Label string
	Text  string
}

var Presets = []Preset{
	{Label: "简体中文", Text: "月光如流水一般，静静地泻在这一片叶子和花上。"},
	{Label: "繁體中文", Text: "月光如流水一般，靜靜地瀉在這一片葉子和花上。"},
	{Label: "日本語", Text: "吾輩は猫である。名前はまだ無い。"},
	{Label: "한국어", Text: "나 보기가 역겨워 가실 때에는 말없이 고이 보내드리오리다."},
	{Label: "English", Text: "Shall I compare thee to a summer's day?"},
	{Label: "Français", Text: "Demain, dès l’aube, à l’heure où blanchit la campagne,"},
	{Label: "Deutsch", Text: "Wer reitet so spät durch Nacht und Wind?"},
	{Label: "Español", Text: "En un lugar de la Mancha, de cuyo nombre no quiero acordarme"},
	{Label: "Português", Text: "As armas, e os Barões assinalados,"},
	{Label: "Русский", Text: "Я помню чудное мгновенье: Передо мной явилась ты,"},
	{Label: "العربية", Text: "عَلى قَدْرِ أهْلِ العَزْم تأتي العَزائِمُ وَتأتي علَى قَدْرِ الكِرامِ المَكارمُ"},
	{Label: "हिन्दी", Text: "बुरा जो देखन मैं चला बुरा न मिलिया कोय। जो दिल खोजौं आपना मुझसा बुरा न कोय॥"},
}

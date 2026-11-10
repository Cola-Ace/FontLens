package preview

type Preset struct {
	Label string
	Text  string
}

var Presets = []Preset{
	{Label: "简体中文", Text: "春眠不觉晓，处处闻啼鸟。夜来风雨声，花落知多少。"},
	{Label: "繁體中文", Text: "春眠不覺曉，處處聞啼鳥。夜來風雨聲，花落知多少。"},
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

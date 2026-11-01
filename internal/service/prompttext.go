package service

import (
	"strings"

	"snzstudio/internal/lang"
)

// promptText is every sentence the app writes for the LLM or into a file it
// generates, in one processing language. It keeps both versions side by side so
// a change to one is seen next to the other. The assistant chat's system prompt
// (chat.go) is not here: it is English in both languages, and the model answers
// in whatever language the user writes in.
type promptText struct {
	humanSpeaker     string
	assistantSpeaker string
	systemSpeaker    string

	// Multi-agent turn. Format strings are noted with their arguments.
	turnRoster        string // names
	reminderOnlySelf  string // speaker
	reminderNoName    string
	reminderReact     string
	reminderLength    string
	reminderDirective string
	// reminderLanguage answers TASK-107: a model handed Japanese scenes and roles
	// still opened in English when nothing named the language.
	reminderLanguage string
	stateHeader      string
	turnCue          string // speaker, speaker

	// materialHeader states what the material is for before the scene and the role
	// arrive: the model reads the system prompt top to bottom, and material with no
	// framing reads as the persona (a reference document answered as an assistant
	// would) — the drift the section exists to avoid.
	materialHeader   string
	materialProject  string // title, excerpt
	materialMemory   string // label, excerpt
	materialDocument string // label, passages

	// conclusionSystem separates decided from open, and keeps an objection that a
	// participant raised because its role told it to (a critic, a devil's advocate)
	// out of the decisions unless someone else took it up. Without that line a small
	// model reports the last strong objection as the outcome.
	conclusionSystem     string
	conclusionHumanRole  string
	conclusionNoRole     string
	conclusionRoster     string
	conclusionTranscript string
	conclusionRequest    string

	summarySystem       string
	summaryNoneYet      string
	summaryExisting     string
	summaryRecent       string
	summaryRequest      string
	summaryFallbackPast string
	summaryFallbackNow  string
	titleSystem         string
	titleRecent         string

	memoryExtractionSystem string

	organizerSystem       string
	organizerMemories     string
	organizerInstructions []string
	organizerNothing      string
	organizerDuplicate    string // existing memory id
	organizerFromSummary  string
	organizerNoChanges    string
	organizerProposed     string

	reviewSystem []string

	imageDescription string

	exportProject      string // title
	exportGeneratedAt  string // time
	exportScene        string
	exportUnset        string
	exportStateShared  string
	exportState        string
	exportTurnRule     string
	exportRoster       string
	exportNoRoster     string
	exportTranscript   string
	exportNoMessages   string
	exportEmptyMessage string
	exportRemoved      string
	ruleRoundRobin     string
	ruleManual         string
	ruleFacilitator    string // facilitator
	ruleNoFacilitator  string
	ruleWeighted       string
	ruleWeightedNamed  string // facilitator
}

func promptTextFor(language lang.Language) *promptText {
	if language == lang.English {
		return &englishPromptText
	}
	return &japanesePromptText
}

var japanesePromptText = promptText{
	humanSpeaker:     "ユーザー",
	assistantSpeaker: "アシスタント",
	systemSpeaker:    "システム",

	turnRoster:        "この会話の参加者: %s",
	reminderOnlySelf:  "あなたは「%s」としてのみ発言する。他の参加者の発言や動作を代筆しない。1 回の発言に複数人分の会話を入れない。",
	reminderNoName:    "発言の先頭に自分の名前や記号を付けない。本文だけを書く。",
	reminderReact:     "直前の発言のどこに反応しているかが分かるように述べる。同意するだけで終わらせず、自分の立場から具体的に述べる。",
	reminderLength:    "発言の長さは場面設定の指定に従う。指定がなければ簡潔にまとめる。",
	reminderDirective: "次に話してほしい相手がいるときだけ、発言の最後の行に [次: 表示名] と書く（複数なら読点で区切る）。いなければ書かない。",
	reminderLanguage:  "発言はすべて日本語で書く。他の参加者が別の言語で話しても、日本語で続ける。",
	stateHeader:       "【現在の状態】\n",
	turnCue:           "（進行）次は「%s」の番です。%s として発言してください。",

	materialHeader: "【プロジェクト資料】\n" +
		"以下はこの会話が属するプロジェクトの資料である。資料は話者の役割・口調・立場を変えない。会話の流れで必要になったときだけ、自分の役割のまま自然に使う。資料を要約したり、アシスタントとして解説したりしない。",
	materialProject:  "[プロジェクト] %s\n%s",
	materialMemory:   "[メモリ] %s: %s",
	materialDocument: "[ドキュメント] %s\n%s",

	conclusionSystem: strings.Join([]string{
		"あなたは議論の結論を抜き出す専用のアシスタントです。",
		"会話は、役割を与えられた複数の参加者と、それを見ている人間（ユーザー）の発言です。",
		"誰が何を言ったかを並べるのではなく、会話の結果として何が決まったか、何が決まっていないかを書いてください。",
		"",
		"- 決定: 参加者が合意した判断、またはユーザーが採用した判断。それぞれに短い理由を添える。",
		"- 未決: 意見が分かれたまま、または結論が出ていない論点。",
		"- 参加者が役割（反対役・批判役・懐疑役など）に沿って出した反論や懸念は、他の参加者が同意したかユーザーが採用した場合を除き、決定に入れない。扱うなら未決に書く。",
		"- ユーザーが却下した案は決定に入れない。",
		"- 会話に無い推測や一般論を足さない。",
		"- 決定が無ければ「決定」の下に「- なし」と書く。未決も同じ。",
		"",
		"出力は次の形式だけにしてください。前置きや締めの文は書かないでください。",
		"決定:",
		"- （判断）— （理由）",
		"未決:",
		"- （論点）",
	}, "\n"),
	conclusionHumanRole:  "会話を見ている人間。この人の発言は採用・却下の判断として扱う",
	conclusionNoRole:     "（役割の指定なし）",
	conclusionRoster:     "参加者と役割:\n",
	conclusionTranscript: "会話:\n",
	conclusionRequest:    "この会話で何が決まり、何が未決かを、指定の形式で書いてください。",

	summarySystem: strings.Join([]string{
		"あなたは会話要約専用のアシスタントです。",
		"会話全体の継続に必要な内容だけを自然な日本語で要約してください。",
		"雑談や冗長な表現は落とし、決定事項、未解決事項、重要な前提、直近の変更だけを残してください。",
		"箇条書きではなく、2から5文程度の短い自然文でまとめてください。",
	}, "\n"),
	summaryNoneYet:      "既存の要約はありません。",
	summaryExisting:     "既存の要約:\n",
	summaryRecent:       "直近の会話:\n",
	summaryRequest:      "上記を踏まえて、今後の会話継続に使える自然な日本語の要約を更新してください。",
	summaryFallbackPast: "これまでの要約:\n",
	summaryFallbackNow:  "直近のやり取り:\n",
	titleSystem: strings.Join([]string{
		"あなたは会話タイトル生成専用のアシスタントです。",
		"会話の内容を表す短い日本語タイトルだけを返してください。",
		"長さは 8 文字から 28 文字程度。",
		"引用符、接頭辞、説明文、句点は付けないでください。",
	}, "\n"),
	titleRecent: "直近の会話:\n",

	memoryExtractionSystem: strings.Join([]string{
		"あなたは project memory 抽出専用アシスタントです。",
		"ユーザーが『覚えて』と明示したので、直前の会話から project をまたいで有用な durable memory を 1 件だけ抽出してください。",
		"返答は JSON のみとし、形式は {\"kind\":\"semantic|procedural|episodic\",\"content\":\"...\"} または {\"skip\":true,\"reason\":\"...\"} にしてください。",
		"content は 1 から 2 文の短い日本語にしてください。",
		"雑談や一時的な話題は保存しないでください。",
	}, "\n"),

	organizerSystem: strings.Join([]string{
		"あなたは project memory の整理専用アシスタントです。",
		"semantic / procedural / episodic の区別を守り、重複統合、書き換え、不要候補の削除、新規 memory 候補の作成だけを提案してください。",
		"必ず JSON のみを返してください。",
	}, "\n"),
	organizerMemories: "現在の memories:",
	organizerInstructions: []string{
		"次の JSON だけを返してください。",
		`{ "summary": "short summary", "changes": [`,
		`  { "action": "create"|"update"|"remove", "memoryId": "existing id for update/remove", "kind": "semantic|procedural|episodic", "title": "title", "content": "content", "reason": "reason" }`,
		"] }",
		"更新は既存 memory の整理に限定し、無意味な全面書き換えは避けてください。",
		"locked=true の memory は update/remove しないでください。",
		"雑談は memory にしないでください。",
		"changes は最大 8 件に抑えてください。",
	},
	organizerNothing:     "整理対象の memory と chat summary がありません。",
	organizerDuplicate:   "同一内容の memory (%s) と重複しています。",
	organizerFromSummary: "直近の chat summary から継続判断に役立つ経緯を 1 件だけ補完します。",
	organizerNoChanges:   "整頓対象は見つかりませんでした。",
	organizerProposed:    "重複統合と recent summary 由来の補完候補を作成しました。",

	reviewSystem: []string{
		"あなたは創作文レビュー専用の編集者です。",
		"与えられた文章を書き直さず、レビューだけを返してください。",
		"日本語で、短く具体的に指摘してください。",
		"特に次を見てください: 設定整合、人物の一貫性、時系列、用語ぶれ、文体の不自然さ、冗長さ、説明過多。",
		"大きな問題がなければ、その旨を述べたうえで軽い改善提案だけを返してください。",
		"出力は markdown で、`Overall`、`Issues`、`Suggestions` の 3 セクションにしてください。",
	},

	imageDescription: `この画像を、あとでキーワード検索とチャットの参考資料として使うための説明文にしてください。

- 日本語で、簡潔に書く。
- 写っている物・人物の様子・場所、画像内の文字 (読める範囲でそのまま)、図や表の構造、固有名を優先して書く。
- 写っていないこと、読み取れないことは推測で書かない。
- 前置きや結びの挨拶は書かず、説明文だけを返す。`,

	exportProject:      "- プロジェクト: %s\n",
	exportGeneratedAt:  "- 出力日時: %s\n",
	exportScene:        "\n## 場面設定\n\n",
	exportUnset:        "（未設定）",
	exportStateShared:  "### 共通\n\n",
	exportState:        "\n## 現在の状態\n\n",
	exportTurnRule:     "\n## ターン進行ルール\n\n",
	exportRoster:       "\n## 編成\n\n",
	exportNoRoster:     "（参加者なし）\n",
	exportTranscript:   "\n## 会話\n",
	exportNoMessages:   "\n（発言なし）\n",
	exportEmptyMessage: "（空の発言）\n",
	exportRemoved:      "（除籍済み）",
	ruleRoundRobin:     "round_robin（編成順に回す）",
	ruleManual:         "manual（1 ターンごとに発言者を指名する）",
	ruleFacilitator:    "facilitator_alternating（進行役「%s」と他の参加者が交互に発言する）",
	ruleNoFacilitator:  "facilitator_alternating（進行役が編成に居ないため、編成順に回す）",
	ruleWeighted:       "weighted（呼びかけと沈黙の長さで次の話者を決める）",
	ruleWeightedNamed:  "weighted（呼びかけと沈黙の長さで次の話者を決める。進行役「%s」）",
}

var englishPromptText = promptText{
	humanSpeaker:     "User",
	assistantSpeaker: "Assistant",
	systemSpeaker:    "System",

	turnRoster:        "Participants in this conversation: %s",
	reminderOnlySelf:  "Speak only as \"%s\". Do not write what the other participants say or do. Do not put more than one person's lines in a single utterance.",
	reminderNoName:    "Do not begin your utterance with your name or a symbol. Write only the utterance itself.",
	reminderReact:     "Make it clear which part of the previous utterance you are responding to. Do not stop at agreeing; speak concretely from your own position.",
	reminderLength:    "Follow the scene's instructions on length. If it gives none, keep the utterance brief.",
	reminderDirective: "Only when you want a particular participant to speak next, write [Next: display name] on the last line of your utterance (separate several names with commas). Otherwise leave it out.",
	reminderLanguage:  "Write every utterance in English. Keep writing in English even if another participant speaks a different language.",
	stateHeader:       "【Current state】\n",
	turnCue:           "(Moderator) It is now \"%s\"'s turn. Speak as %s.",

	materialHeader: "【Project material】\n" +
		"The following is material from the project this conversation belongs to. It does not change the speaker's role, tone or position. Use it only when the conversation calls for it, naturally and in your own role. Do not summarize it or explain it as an assistant would.",
	materialProject:  "[Project] %s\n%s",
	materialMemory:   "[Memory] %s: %s",
	materialDocument: "[Document] %s\n%s",

	conclusionSystem: strings.Join([]string{
		"You are an assistant that only extracts the conclusions of a discussion.",
		"The conversation consists of utterances by several participants who were given roles, and by the human (the user) watching them.",
		"Do not list who said what. Write what the conversation decided and what it left undecided.",
		"",
		"- Decided: a judgment the participants agreed on or the user adopted, each with a short reason.",
		"- Open: a point where opinions stayed divided or no conclusion was reached.",
		"- Leave out of Decided any objection or concern a participant raised because of its role (opponent, critic, skeptic and so on), unless another participant agreed or the user adopted it. If you include it, put it under Open.",
		"- Leave proposals the user rejected out of Decided.",
		"- Do not add guesses or generalities that are not in the conversation.",
		"- If nothing was decided, write \"- None\" under Decided. The same goes for Open.",
		"",
		"Output only the following format, with no introduction or closing remarks.",
		"Decided:",
		"- (judgment) — (reason)",
		"Open:",
		"- (point)",
	}, "\n"),
	conclusionHumanRole:  "the human watching the conversation; treat what this person says as adopting or rejecting proposals",
	conclusionNoRole:     "(no role given)",
	conclusionRoster:     "Participants and roles:\n",
	conclusionTranscript: "Conversation:\n",
	conclusionRequest:    "Write what this conversation decided and what it left open, in the format specified.",

	summarySystem: strings.Join([]string{
		"You are an assistant that only summarizes conversations.",
		"Summarize, in natural English, only what is needed to continue the conversation.",
		"Drop small talk and redundancy. Keep decisions, open issues, important assumptions and recent changes only.",
		"Write a short passage of about 2 to 5 natural sentences, not bullet points.",
	}, "\n"),
	summaryNoneYet:      "There is no existing summary.",
	summaryExisting:     "Existing summary:\n",
	summaryRecent:       "Recent conversation:\n",
	summaryRequest:      "Based on the above, update the summary in natural English so it can be used to continue the conversation.",
	summaryFallbackPast: "Summary so far:\n",
	summaryFallbackNow:  "Recent exchanges:\n",
	titleSystem: strings.Join([]string{
		"You are an assistant that only generates conversation titles.",
		"Return only a short English title describing the conversation.",
		"Use at most 6 words.",
		"Do not add quotation marks, prefixes, explanations or a final period.",
	}, "\n"),
	titleRecent: "Recent conversation:\n",

	memoryExtractionSystem: strings.Join([]string{
		"You are an assistant that only extracts project memories.",
		"The user explicitly asked you to remember something, so extract exactly one durable memory from the conversation so far that will stay useful across the project.",
		"Reply with JSON only, in the form {\"kind\":\"semantic|procedural|episodic\",\"content\":\"...\"} or {\"skip\":true,\"reason\":\"...\"}.",
		"Write content as 1 to 2 short sentences in English.",
		"Do not save small talk or passing topics.",
	}, "\n"),

	organizerSystem: strings.Join([]string{
		"You are an assistant that only organizes project memories.",
		"Keep the distinction between semantic, procedural and episodic. Propose only merging duplicates, rewriting, removing what is no longer needed, and creating new memory candidates.",
		"Always return JSON only.",
	}, "\n"),
	organizerMemories: "Current memories:",
	organizerInstructions: []string{
		"Return only the following JSON. Write summary, title, content and reason in English.",
		`{ "summary": "short summary", "changes": [`,
		`  { "action": "create"|"update"|"remove", "memoryId": "existing id for update/remove", "kind": "semantic|procedural|episodic", "title": "title", "content": "content", "reason": "reason" }`,
		"] }",
		"Limit updates to organizing existing memories, and avoid pointless wholesale rewrites.",
		"Do not update or remove a memory with locked=true.",
		"Do not turn small talk into memories.",
		"Keep changes to at most 8 entries.",
	},
	organizerNothing:     "There are no memories or chat summaries to organize.",
	organizerDuplicate:   "Duplicates memory %s, which has the same content.",
	organizerFromSummary: "Adds one memory of useful background from the recent chat summaries.",
	organizerNoChanges:   "Found nothing to organize.",
	organizerProposed:    "Proposed merging duplicates and adding a candidate from the recent summaries.",

	reviewSystem: []string{
		"You are an editor who only reviews creative writing.",
		"Do not rewrite the text you are given. Return only a review.",
		"Point things out in English, briefly and concretely.",
		"Look especially at: consistency with the setting, consistency of characters, chronology, inconsistent terms, unnatural style, redundancy and over-explanation.",
		"If there are no major problems, say so and offer only light suggestions.",
		"Output markdown with three sections: `Overall`, `Issues` and `Suggestions`.",
	},

	imageDescription: `Describe this image so the description can later serve as material for keyword search and for chat.

- Write in English, briefly.
- Give priority to the objects, how people look, the place, any text in the image (verbatim as far as it is legible), the structure of diagrams and tables, and proper names.
- Do not guess at what is not shown or cannot be read.
- Return only the description, with no greeting or closing remark.`,

	exportProject:      "- Project: %s\n",
	exportGeneratedAt:  "- Exported at: %s\n",
	exportScene:        "\n## Scene\n\n",
	exportUnset:        "(not set)",
	exportStateShared:  "### Shared\n\n",
	exportState:        "\n## Current state\n\n",
	exportTurnRule:     "\n## Turn rule\n\n",
	exportRoster:       "\n## Roster\n\n",
	exportNoRoster:     "(no participants)\n",
	exportTranscript:   "\n## Conversation\n",
	exportNoMessages:   "\n(no utterances)\n",
	exportEmptyMessage: "(empty utterance)\n",
	exportRemoved:      " (removed)",
	ruleRoundRobin:     "round_robin (in roster order)",
	ruleManual:         "manual (the speaker is nominated each turn)",
	ruleFacilitator:    "facilitator_alternating (the facilitator \"%s\" alternates with the other participants)",
	ruleNoFacilitator:  "facilitator_alternating (in roster order, because the facilitator is not on the roster)",
	ruleWeighted:       "weighted (the next speaker follows from calls and how long each has been silent)",
	ruleWeightedNamed:  "weighted (the next speaker follows from calls and how long each has been silent; facilitator \"%s\")",
}

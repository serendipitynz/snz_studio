import { DocumentCategory } from "./types.js";

const CATEGORIES: DocumentCategory[] = ["world", "character", "rule", "plot", "timeline", "index", "story", "misc"];

const CATEGORY_HINTS: Array<{ category: DocumentCategory; patterns: RegExp[] }> = [
  {
    category: "story",
    patterns: [
      /note[_-]?chapter/iu,
      /chapter\s*\d+/iu,
      /第\s*\d+\s*(?:話|章)/u,
      /^#\s*第\s*\d+\s*(?:話|章)/mu,
      /\*\*第[一二三四五六七八九十]+部/u
    ]
  },
  {
    category: "plot",
    patterns: [/プロット/u, /物語構想/u, /各章内容設計/u, /物語的機能/u, /章末の状態/u, /起きること/u]
  },
  {
    category: "timeline",
    patterns: [/時系列/u, /\bD\+?\d+\b/u, /全体の時間幅/u, /この時点での情報保有/u, /前後関係/u]
  },
  {
    category: "index",
    patterns: [/索引/u, /インデックス/u, /正史確定/u, /暫定確定/u, /保留/u, /用語索引/u, /地理索引/u]
  },
  {
    category: "character",
    patterns: [/主要人物/u, /人物メモ/u, /キャラクター/u, /主人公候補/u, /年齢/u, /性格/u, /口調/u, /家族構成/u]
  },
  {
    category: "rule",
    patterns: [/基礎ルール/u, /本文運用メモ/u, /運用メモ/u, /恒常原則/u, /文体制限/u, /方針/u, /優先/u, /不要/u]
  },
  {
    category: "world",
    patterns: [/世界設定/u, /世界観/u, /創世神話/u, /神々/u, /国家/u, /魔法体系/u, /生活世界設定/u, /種族/u]
  }
];

const CATEGORY_ORDER: DocumentCategory[] = ["story", "plot", "timeline", "index", "character", "rule", "world", "misc"];

export function isDocumentCategory(value: string): value is DocumentCategory {
  return CATEGORIES.includes(value as DocumentCategory);
}

export function inferDocumentCategory(input: {
  fileName?: string | null;
  title?: string | null;
  note?: string | null;
  contentText?: string | null;
  derivedText?: string | null;
}) {
  const sample = [input.fileName, input.title, input.note, input.contentText, input.derivedText]
    .filter(Boolean)
    .join("\n")
    .slice(0, 5000);

  if (!sample.trim()) {
    return "misc" as const;
  }

  const scores = new Map<DocumentCategory, number>();
  for (const category of CATEGORY_ORDER) {
    scores.set(category, 0);
  }

  for (const { category, patterns } of CATEGORY_HINTS) {
    for (const pattern of patterns) {
      const matched = sample.match(pattern);
      if (matched) {
        scores.set(category, (scores.get(category) ?? 0) + 1);
      }
    }
  }

  if (/^#\s*第\s*\d+\s*(?:話|章)/mu.test(sample)) {
    scores.set("story", (scores.get("story") ?? 0) + 3);
  }

  if (/\*\*視点\*\*|字数目安|物語的機能|章末の状態/u.test(sample)) {
    scores.set("plot", (scores.get("plot") ?? 0) + 3);
  }

  let bestCategory: DocumentCategory = "misc";
  let bestScore = 0;

  for (const category of CATEGORY_ORDER) {
    const score = scores.get(category) ?? 0;
    if (score > bestScore) {
      bestCategory = category;
      bestScore = score;
    }
  }

  return bestScore > 0 ? bestCategory : "misc";
}

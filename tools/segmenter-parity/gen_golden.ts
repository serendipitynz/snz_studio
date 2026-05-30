// Generates internal/search/testdata/golden.json: the JS reference output for a
// corpus of inputs, used by searchtext_test.go to assert the Go port matches the
// original Node implementation exactly.
// Run from repo root:  pnpm exec tsx tools/segmenter-parity/gen_golden.ts
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import TinySegmenter from "tiny-segmenter";
import {
  buildSearchText,
  tokenizeSearchTerms,
  toFtsQuery,
} from "../../backend/src/lib/searchText";

const segmenter = new TinySegmenter();
const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "../..");

// Curated edge cases covering the JS code paths: mixed JP/EN, full-width forms,
// punctuation, numbers/dates, single kana, stopwords, katakana, empty/whitespace.
const curated: string[] = [
  "",
  "   ",
  "\n\t  \r\n",
  "の",
  "です",
  "して",
  "これ",
  "お願いします",
  "日本語 test",
  "TypeScript と React の設定",
  "API endpoint v2.12.0",
  "東京都の天気はどうですか",
  "チャット「第3話・作業」の内容を確認して",
  "設定確認",
  "プロット相談",
  "翻訳をお願いします",
  "ＡＢＣ１２３ と ABC123",
  "こんにちは、世界。",
  "―― 区切り線 ――",
  "2026年5月30日に第1部プロットを書いた",
  "主人公は街を歩いていた。彼女は静かに微笑んだ。",
  "カタカナ テスト コンピュータ サーバー",
  "snz_studio という project-name のテスト",
  "全角　スペース　区切り",
  "改行\nを含む\nテキスト",
  "世界設定",
  "主要人物",
  "基礎ルール",
  "正史インデックス",
  "物語構想",
];

const inputs: { label: string; input: string }[] = curated.map((input, i) => ({
  label: `curated-${i}`,
  input,
}));

// Add every sample-docs file as a full-content segmentation test.
const sampleDir = path.join(repoRoot, "sample-docs");
for (const name of readdirSync(sampleDir).sort()) {
  if (!name.endsWith(".md")) continue;
  const content = readFileSync(path.join(sampleDir, name), "utf8");
  inputs.push({ label: `sample:${name}`, input: content });
}

const golden = inputs.map(({ label, input }) => ({
  label,
  input,
  segments: segmenter.segment(input),
  buildSearchText: buildSearchText(input),
  terms: tokenizeSearchTerms(input),
  ftsQuery: toFtsQuery(input),
}));

const out = path.join(repoRoot, "internal/search/testdata/golden.json");
writeFileSync(out, JSON.stringify(golden, null, 2) + "\n", "utf8");
console.error(`wrote ${out} (${golden.length} cases)`);

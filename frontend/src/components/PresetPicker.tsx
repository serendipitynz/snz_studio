import { useEffect, useMemo, useRef, useState } from "react";
import { api, MultiAgentPreset, MultiAgentPresetSelection } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { Button, ErrorText, Field, Row, Select, Subtle } from "../styles/ui";

// The picker value that stands for the preset read from a file: it lives beside
// the bundled ids in the same select, so it must be a value no bundled id uses.
const IMPORTED_PRESET_CHOICE = "__file__";

const GROUP_ORDER = ["discussion", "drama", "hosted", "pair"];

// The bundled list is fixed for the life of the process, so it is fetched once
// and kept here rather than per mount: the picker is mounted and unmounted as
// the chat kind is switched and as the organisation panel opens.
let bundledRequest: Promise<MultiAgentPreset[]> | null = null;

function loadBundledPresets(): Promise<MultiAgentPreset[]> {
  if (!bundledRequest) {
    bundledRequest = api
      .listMultiAgentPresets()
      .then((response) => response.presets)
      .catch((error) => {
        bundledRequest = null;
        throw error;
      });
  }
  return bundledRequest;
}

export interface PresetChoice {
  // What the request carries: a bundled id, or the imported preset inline.
  selection: MultiAgentPresetSelection;
  preset: MultiAgentPreset;
}

interface PresetPickerProps {
  disabled?: boolean;
  onChange: (choice: PresetChoice | null) => void;
}

// PresetPicker is the preset half of docs/multi-agent-chat-design.md §6: the
// bundled presets grouped in one select, plus a preset read from a local JSON
// file. Both the creation form and the organisation panel show it, and both send
// what it reports through onChange — the two routes take the same pair of fields.
export function PresetPicker(props: PresetPickerProps) {
  const { t } = useLanguage();
  const fileRef = useRef<HTMLInputElement | null>(null);
  const [presets, setPresets] = useState<MultiAgentPreset[]>([]);
  const [choice, setChoice] = useState("");
  const [imported, setImported] = useState<MultiAgentPreset | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    loadBundledPresets()
      .then((loaded) => {
        if (active) {
          setPresets(loaded);
        }
      })
      .catch((nextError) => {
        if (active) {
          setError(nextError instanceof Error ? nextError.message : t("preset.loadError"));
        }
      });
    return () => {
      active = false;
    };
  }, [t]);

  const groups = useMemo(() => {
    const byGroup = new Map<string, MultiAgentPreset[]>();
    for (const preset of presets) {
      const group = GROUP_ORDER.includes(preset.group) ? preset.group : "other";
      byGroup.set(group, [...(byGroup.get(group) ?? []), preset]);
    }
    return [...GROUP_ORDER, "other"]
      .filter((group) => byGroup.has(group))
      .map((group) => [group, byGroup.get(group) ?? []] as const);
  }, [presets]);

  const selected = choice === IMPORTED_PRESET_CHOICE ? imported : presets.find((preset) => preset.id === choice) ?? null;

  function groupLabel(group: string): string {
    const key = `preset.group.${group}` as MessageKey;
    return t(key) === key ? t("preset.group.other") : t(key);
  }

  function report(value: string, importedPreset: MultiAgentPreset | null) {
    if (value === IMPORTED_PRESET_CHOICE) {
      props.onChange(importedPreset ? { selection: { preset: importedPreset }, preset: importedPreset } : null);
      return;
    }
    const preset = presets.find((candidate) => candidate.id === value);
    props.onChange(preset ? { selection: { presetId: preset.id }, preset } : null);
  }

  function handleChoose(value: string) {
    setChoice(value);
    report(value, imported);
  }

  // Only the shape the picker needs is checked here; the server validates the
  // preset in full and its message is shown if it refuses the file.
  async function handleImportFile(file: File | undefined) {
    if (!file) {
      return;
    }
    setError("");
    try {
      const parsed = JSON.parse(await file.text()) as Partial<MultiAgentPreset> | null;
      if (!parsed || typeof parsed.title !== "string" || !Array.isArray(parsed.participants)) {
        throw new Error("not a preset");
      }
      const preset = parsed as MultiAgentPreset;
      setImported(preset);
      setChoice(IMPORTED_PRESET_CHOICE);
      report(IMPORTED_PRESET_CHOICE, preset);
    } catch {
      setError(t("preset.importError"));
    }
  }

  return (
    <>
      <Field>
        {t("preset.label")}
        <Select value={choice} disabled={props.disabled} onChange={(event) => handleChoose(event.target.value)}>
          <option value="">{t("preset.none")}</option>
          {imported ? <option value={IMPORTED_PRESET_CHOICE}>{t("preset.imported", { title: imported.title })}</option> : null}
          {groups.map(([group, items]) => (
            <optgroup key={group} label={groupLabel(group)}>
              {items.map((preset) => (
                <option key={preset.id} value={preset.id}>
                  {preset.title}
                </option>
              ))}
            </optgroup>
          ))}
        </Select>
      </Field>
      {selected ? (
        <Subtle style={{ margin: 0 }}>
          {selected.description}{" "}
          {t("preset.summary", { count: selected.participants.length, turnRule: selected.turnRule })}
        </Subtle>
      ) : null}
      <Row style={{ alignItems: "center", gap: 10 }}>
        <Button type="button" variant="ghost" disabled={props.disabled} onClick={() => fileRef.current?.click()}>
          {t("preset.import")}
        </Button>
      </Row>
      <Subtle style={{ margin: 0 }}>{t("preset.hint")}</Subtle>
      {error ? <ErrorText>{error}</ErrorText> : null}
      <input
        ref={fileRef}
        type="file"
        accept=".json,application/json"
        style={{ display: "none" }}
        onChange={(event) => {
          void handleImportFile(event.target.files?.[0]);
          event.target.value = "";
        }}
      />
    </>
  );
}

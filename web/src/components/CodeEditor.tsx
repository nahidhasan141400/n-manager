import { useEffect, useRef, useState } from "react";
import { StreamLanguage } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { nginx } from "@codemirror/legacy-modes/mode/nginx";
import { oneDark } from "@codemirror/theme-one-dark";
import { basicSetup } from "codemirror";

export function CodeEditor({
  value,
  onChange,
  readOnly = false,
}: {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const theme = useThemeName();

  useEffect(() => {
    if (!host.current) return;
    const extensions = [
      basicSetup,
      EditorView.lineWrapping,
      StreamLanguage.define(nginx),
      EditorView.editable.of(!readOnly),
      EditorView.theme({
        "&": { height: "420px", border: "1px solid var(--border)", borderRadius: "6px", backgroundColor: "var(--input)", fontSize: "13px" },
        ".cm-scroller": { overflow: "auto", fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" },
        ".cm-gutters": { backgroundColor: "transparent", borderRight: "1px solid var(--border)" },
        ".cm-content": { caretColor: "var(--text)" },
      }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) onChangeRef.current?.(update.state.doc.toString());
      }),
    ];
    if (theme === "dark") extensions.push(oneDark);
    const view = new EditorView({
      parent: host.current,
      state: EditorState.create({ doc: value, extensions }),
    });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, [theme, readOnly]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (current !== value) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
    }
  }, [value]);

  return <div ref={host} className="overflow-hidden rounded-md" />;
}

function useThemeName() {
  const [theme, setTheme] = useState(() => document.documentElement.dataset.theme || "light");
  useEffect(() => {
    const observer = new MutationObserver(() => setTheme(document.documentElement.dataset.theme || "light"));
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => observer.disconnect();
  }, []);
  return theme;
}

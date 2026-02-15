"use client";

import { useState, useRef, useEffect } from "react";
import { Send, AlertTriangle, ChevronDown, RefreshCw } from "lucide-react";
import { queryKnowledgeBase, ApiError } from "@/lib/api";
import type { QueryResponse, Citation, DebugInfo } from "@/lib/types";
import { cn, shortId } from "@/lib/utils";

interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
  data?: QueryResponse;
  error?: string;
}

const EXAMPLE_QUESTIONS = [
  "What is the password policy?",
  "How do I submit an expense report?",
  "What are the remote work requirements?",
  "What is the incident response procedure?",
];

export default function ChatPage() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [debug, setDebug] = useState(false);
  const chatRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (chatRef.current) {
      chatRef.current.scrollTop = chatRef.current.scrollHeight;
    }
  }, [messages]);

  const sendMessage = async (question: string) => {
    if (!question.trim() || loading) return;

    const userMsg: Message = {
      id: crypto.randomUUID(),
      role: "user",
      content: question.trim(),
    };

    setMessages((prev) => [...prev, userMsg]);
    setInput("");
    setLoading(true);

    try {
      const res = await queryKnowledgeBase(question.trim(), debug);
      const assistantMsg: Message = {
        id: crypto.randomUUID(),
        role: "assistant",
        content: res.answer,
        data: res,
      };
      setMessages((prev) => [...prev, assistantMsg]);
    } catch (err: unknown) {
      let errorMsg = "Failed to get answer";
      if (err instanceof ApiError) {
        if (err.status === 502) {
          errorMsg = "The AI service is temporarily unavailable. Please try again in a moment.";
        } else if (err.status === 400) {
          errorMsg = err.message;
        } else {
          errorMsg = err.message;
        }
      } else if (err instanceof Error) {
        errorMsg = err.message;
      }

      const errMessage: Message = {
        id: crypto.randomUUID(),
        role: "assistant",
        content: "",
        error: errorMsg,
      };
      setMessages((prev) => [...prev, errMessage]);
    } finally {
      setLoading(false);
      inputRef.current?.focus();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage(input);
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="px-6 py-3 border-b border-border flex items-center justify-between bg-surface">
        <h1 className="text-lg font-semibold">Chat</h1>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={debug}
            onChange={(e) => setDebug(e.target.checked)}
            className="sr-only"
          />
          <div
            className={cn(
              "w-8 h-4 rounded-full relative transition-colors",
              debug ? "bg-accent-dim" : "bg-border"
            )}
          >
            <div
              className={cn(
                "w-3.5 h-3.5 rounded-full absolute top-0.5 transition-all",
                debug
                  ? "left-4 bg-accent"
                  : "left-0.5 bg-text-dim"
              )}
            />
          </div>
          <span className="text-xs text-text-dim">Debug</span>
        </label>
      </div>

      {/* Messages */}
      <div ref={chatRef} className="flex-1 overflow-y-auto p-6 space-y-6">
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <h2 className="text-xl font-semibold mb-2">Ask your knowledge base</h2>
            <p className="text-text-dim text-sm max-w-md mb-6">
              Ask questions about company policies, procedures, and guidelines.
              Answers are grounded in ingested documents with citations.
            </p>
            <div className="flex flex-wrap gap-2 justify-center max-w-lg">
              {EXAMPLE_QUESTIONS.map((q) => (
                <button
                  key={q}
                  onClick={() => sendMessage(q)}
                  className="px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm hover:border-accent transition-colors"
                >
                  {q}
                </button>
              ))}
            </div>
          </div>
        ) : (
          messages.map((msg) => (
            <div key={msg.id} className="max-w-3xl mx-auto">
              {msg.role === "user" ? (
                <div className="flex justify-end">
                  <div className="bg-accent-dim text-white rounded-xl rounded-br-sm px-4 py-3 max-w-[70%] text-sm leading-relaxed">
                    {msg.content}
                  </div>
                </div>
              ) : msg.error ? (
                <div className="bg-red-500/10 border border-red-500/30 rounded-xl px-4 py-3 flex items-start gap-2">
                  <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                  <div>
                    <p className="text-sm text-red-400">{msg.error}</p>
                    <button
                      onClick={() => {
                        // Find the last user message and retry
                        const lastUser = [...messages]
                          .reverse()
                          .find((m) => m.role === "user");
                        if (lastUser) {
                          setMessages((prev) =>
                            prev.filter((m) => m.id !== msg.id)
                          );
                          sendMessage(lastUser.content);
                        }
                      }}
                      className="flex items-center gap-1 text-xs text-accent hover:underline mt-2"
                    >
                      <RefreshCw className="w-3 h-3" />
                      Retry
                    </button>
                  </div>
                </div>
              ) : (
                <AssistantMessage data={msg.data!} debug={debug} />
              )}
            </div>
          ))
        )}

        {loading && (
          <div className="max-w-3xl mx-auto">
            <div className="bg-surface border border-border rounded-xl rounded-bl-sm px-4 py-3">
              <div className="flex items-center gap-2 text-text-dim text-sm">
                <div className="flex gap-1">
                  <span className="w-1.5 h-1.5 bg-text-dim rounded-full animate-bounce" />
                  <span className="w-1.5 h-1.5 bg-text-dim rounded-full animate-bounce [animation-delay:0.2s]" />
                  <span className="w-1.5 h-1.5 bg-text-dim rounded-full animate-bounce [animation-delay:0.4s]" />
                </div>
                Searching documents and generating answer...
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Input */}
      <div className="p-4 border-t border-border bg-surface">
        <div className="max-w-3xl mx-auto flex gap-3 items-end">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask a question..."
            rows={1}
            className="flex-1 px-4 py-2.5 bg-surface-2 border border-border rounded-xl text-sm text-foreground placeholder:text-text-dim focus:outline-none focus:border-accent resize-none min-h-[44px] max-h-[120px]"
            style={{
              height: "auto",
              minHeight: "44px",
            }}
            onInput={(e) => {
              const target = e.target as HTMLTextAreaElement;
              target.style.height = "auto";
              target.style.height =
                Math.min(target.scrollHeight, 120) + "px";
            }}
          />
          <button
            onClick={() => sendMessage(input)}
            disabled={!input.trim() || loading}
            className="w-10 h-10 bg-accent rounded-xl flex items-center justify-center hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors shrink-0"
          >
            <Send className="w-4 h-4 text-background" />
          </button>
        </div>
        <p className="text-center text-xs text-text-dim mt-2">
          pro-rag V1 — Answers are grounded in documents. The system will abstain when evidence is weak.
        </p>
      </div>
    </div>
  );
}

function AssistantMessage({
  data,
  debug,
}: {
  data: QueryResponse;
  debug: boolean;
}) {
  const [showDebug, setShowDebug] = useState(false);

  // Highlight citation markers in the answer
  const highlightedAnswer = data.answer.replace(
    /\[chunk:([^\]]+)\]/g,
    '<span class="text-accent text-xs font-mono">[chunk:$1]</span>'
  );

  return (
    <div className="bg-surface border border-border rounded-xl rounded-bl-sm px-4 py-3 space-y-3">
      {/* Abstain badge */}
      {data.abstained && (
        <span className="inline-block px-2 py-0.5 bg-orange-500/20 text-orange-400 rounded text-xs font-medium">
          ABSTAINED
        </span>
      )}

      {/* Answer */}
      <div
        className="text-sm leading-relaxed whitespace-pre-wrap"
        dangerouslySetInnerHTML={{ __html: highlightedAnswer }}
      />

      {/* Clarifying question */}
      {data.clarifying_question && (
        <p className="text-sm text-orange-400 italic">
          {data.clarifying_question}
        </p>
      )}

      {/* Citations */}
      {data.citations && data.citations.length > 0 && (
        <div className="border-t border-border pt-3">
          <p className="text-xs text-text-dim uppercase tracking-wider mb-2">
            Sources
          </p>
          <div className="space-y-1">
            {data.citations.map((c) => (
              <CitationItem key={c.chunk_id} citation={c} />
            ))}
          </div>
        </div>
      )}

      {/* Debug panel */}
      {debug && data.debug && (
        <details
          className="border-t border-border pt-3"
          open={showDebug}
          onToggle={(e) =>
            setShowDebug((e.target as HTMLDetailsElement).open)
          }
        >
          <summary className="text-xs text-text-dim uppercase tracking-wider cursor-pointer hover:text-foreground">
            Debug Info
          </summary>
          <DebugPanel info={data.debug} />
        </details>
      )}
    </div>
  );
}

function CitationItem({ citation }: { citation: Citation }) {
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 bg-surface-2 rounded text-xs">
      <span className="font-mono text-accent">{shortId(citation.chunk_id)}…</span>
      <span>{citation.title}</span>
      {citation.version_label && (
        <span className="text-text-dim">({citation.version_label})</span>
      )}
    </div>
  );
}

function DebugPanel({ info }: { info: DebugInfo }) {
  return (
    <div className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs font-mono">
      <span className="text-text-dim">Vec candidates</span>
      <span>{info.vec_candidates}</span>
      <span className="text-text-dim">FTS candidates</span>
      <span>{info.fts_candidates}</span>
      <span className="text-text-dim">Merged candidates</span>
      <span>{info.merged_candidates}</span>
      <span className="text-text-dim">Reranker used</span>
      <span>{String(info.reranker_used)}</span>
      <span className="text-text-dim">Reranker skipped</span>
      <span>{String(info.reranker_skipped)}</span>
      {info.reranker_error && (
        <>
          <span className="text-text-dim">Reranker error</span>
          <span className="text-orange-400">{info.reranker_error}</span>
        </>
      )}
      <span className="text-text-dim">Context chunks</span>
      <span>{info.context_chunks}</span>
      <span className="text-text-dim">Context tokens (est)</span>
      <span>{info.context_tokens_est}</span>
      {info.top_scores && info.top_scores.length > 0 && (
        <>
          <span className="text-text-dim">Top scores</span>
          <span>[{info.top_scores.map((s) => s.toFixed(4)).join(", ")}]</span>
        </>
      )}
    </div>
  );
}

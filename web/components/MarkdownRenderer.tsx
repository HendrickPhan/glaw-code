"use client";

import React from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";

export default function MarkdownRenderer({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        code({ className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || "");
          const code = String(children).replace(/\n$/, "");
          const isBlock = code.includes("\n") || className;

          if (match || isBlock) {
            return (
              <div className="relative my-2 rounded-lg overflow-hidden border border-zinc-700/50">
                {match && (
                  <div className="bg-zinc-800 px-3 py-1 text-xs text-zinc-400 border-b border-zinc-700/50">
                    {match[1]}
                  </div>
                )}
                <SyntaxHighlighter
                  style={oneDark as { [key: string]: React.CSSProperties }}
                  language={match?.[1] || "text"}
                  PreTag="div"
                  customStyle={{
                    margin: 0,
                    borderRadius: 0,
                    background: "#1a1a2e",
                    fontSize: "13px",
                  }}
                >
                  {code}
                </SyntaxHighlighter>
                <button
                  onClick={() => navigator.clipboard.writeText(code)}
                  className="absolute top-1 right-1 p-1 rounded text-zinc-500 hover:text-zinc-300 hover:bg-zinc-700/50 transition-colors"
                  title="Copy"
                >
                  <svg
                    width="14"
                    height="14"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                  >
                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                  </svg>
                </button>
              </div>
            );
          }
          return (
            <code
              className="bg-zinc-800 text-emerald-400 px-1.5 py-0.5 rounded text-sm font-mono"
              {...props}
            >
              {children}
            </code>
          );
        },
        p({ children }) {
          return <p className="mb-2 last:mb-0 leading-relaxed">{children}</p>;
        },
        ul({ children }) {
          return <ul className="list-disc pl-5 mb-2 space-y-1">{children}</ul>;
        },
        ol({ children }) {
          return (
            <ol className="list-decimal pl-5 mb-2 space-y-1">{children}</ol>
          );
        },
        li({ children }) {
          return <li className="leading-relaxed">{children}</li>;
        },
        h1({ children }) {
          return (
            <h1 className="text-xl font-bold mt-4 mb-2 text-zinc-100">
              {children}
            </h1>
          );
        },
        h2({ children }) {
          return (
            <h2 className="text-lg font-bold mt-3 mb-2 text-zinc-100">
              {children}
            </h2>
          );
        },
        h3({ children }) {
          return (
            <h3 className="text-base font-semibold mt-2 mb-1 text-zinc-200">
              {children}
            </h3>
          );
        },
        blockquote({ children }) {
          return (
            <blockquote className="border-l-3 border-zinc-600 pl-3 my-2 text-zinc-400 italic">
              {children}
            </blockquote>
          );
        },
        a({ href, children }) {
          return (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:text-blue-300 underline"
            >
              {children}
            </a>
          );
        },
        table({ children }) {
          return (
            <div className="overflow-x-auto my-2">
              <table className="min-w-full border-collapse border border-zinc-700">
                {children}
              </table>
            </div>
          );
        },
        th({ children }) {
          return (
            <th className="border border-zinc-700 px-3 py-1.5 bg-zinc-800 text-left text-sm font-semibold">
              {children}
            </th>
          );
        },
        td({ children }) {
          return (
            <td className="border border-zinc-700 px-3 py-1.5 text-sm">
              {children}
            </td>
          );
        },
        hr() {
          return <hr className="border-zinc-700 my-4" />;
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
}

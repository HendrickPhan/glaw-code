"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { COMMANDS, findCommands, getCommand, CommandSpec } from "@/lib/commands";

export function useCommands(onCommand: (cmd: string) => void) {
  const [isOpen, setIsOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // External clear callback - set by InputBar to sync React state
  const externalClearRef = useRef<(() => void) | null>(null);

  const filteredCommands = findCommands(filter);

  // Reset selection when filtered commands change
  useEffect(() => {
    setSelectedIndex(0);
  }, [filteredCommands.length]);

  // Close on escape
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && isOpen) {
        setIsOpen(false);
        setFilter("");
        setSelectedIndex(0);
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen]);

  const handleInputChange = useCallback((value: string) => {
    // Check if input starts with /
    if (value.startsWith("/")) {
      const cmdPart = value.split(" ")[0];
      setFilter(cmdPart);
      setIsOpen(true);
    } else {
      setIsOpen(false);
      setFilter("");
      setSelectedIndex(0);
    }
  }, []);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!isOpen) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1) % filteredCommands.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 + filteredCommands.length) % filteredCommands.length);
        break;
      case "Tab":
        e.preventDefault();
        if (filteredCommands.length > 0) {
          const selected = filteredCommands[selectedIndex];
          // Replace the command part with the selected command
          const input = inputRef.current?.value || "";
          const parts = input.split(" ");
          parts[0] = selected.cmd;
          const newValue = parts.join(" ");
          setFilter(selected.cmd);
          // Use external clear to update React state with new value
          if (externalClearRef.current) {
            // We need a "setInput" instead of just clear here
            // Use the native setter to update the textarea value
            const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
              window.HTMLTextAreaElement.prototype,
              "value"
            )?.set;
            if (nativeInputValueSetter && inputRef.current) {
              nativeInputValueSetter.call(inputRef.current, newValue);
              inputRef.current.dispatchEvent(new Event("input", { bubbles: true }));
            }
          }
          setSelectedIndex(0);
        }
        break;
      case "Enter":
        if (filteredCommands.length > 0 && !e.shiftKey) {
          e.preventDefault();
          const selected = filteredCommands[selectedIndex];
          onCommand(selected.cmd);
          setIsOpen(false);
          setFilter("");
          setSelectedIndex(0);
          // Clear input via the external React state setter
          if (externalClearRef.current) {
            externalClearRef.current();
          }
        }
        break;
    }
  }, [isOpen, filteredCommands, selectedIndex, onCommand]);

  const selectCommand = useCallback((spec: CommandSpec) => {
    onCommand(spec.cmd);
    setIsOpen(false);
    setFilter("");
    setSelectedIndex(0);
    // Clear input via the external React state setter
    if (externalClearRef.current) {
      externalClearRef.current();
    }
  }, [onCommand]);

  const closePalette = useCallback(() => {
    setIsOpen(false);
    setFilter("");
    setSelectedIndex(0);
  }, []);

  const registerClearCallback = useCallback((clearFn: () => void) => {
    externalClearRef.current = clearFn;
  }, []);

  return {
    isOpen,
    filteredCommands,
    selectedIndex,
    handleInputChange,
    handleKeyDown,
    selectCommand,
    closePalette,
    registerClearCallback,
    inputRef,
  };
}

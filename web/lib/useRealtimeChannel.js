"use client";

import { useEffect } from "react";
import { subscribeRealtimeChannel } from "./realtime.js";

export function useRealtimeChannel(channel, topics, onEvent, options = {}) {
  useEffect(() => {
    if (!channel || typeof onEvent !== "function") return undefined;
    return subscribeRealtimeChannel(channel, topics, onEvent, options);
  }, [channel, onEvent, options, topics]);
}

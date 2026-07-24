import { useCallback, useEffect, useRef, useState } from "react";

import type {
  FactoryEmulatorInstance,
  FactoryEmulatorInstanceState,
} from "../state/factory-emulator-instance";

interface CustomerDemoPlaybackOptions<State, World> {
  readonly instance: FactoryEmulatorInstance<State, World>;
  readonly nextDelayMs: number;
  readonly state: FactoryEmulatorInstanceState<State, World>;
}

function prefersReducedMotion(): boolean {
  return (
    typeof window === "undefined" ||
    typeof window.matchMedia !== "function" ||
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function useReducedMotion(onReduction: () => void): boolean {
  const [reducedMotion, setReducedMotion] = useState(prefersReducedMotion);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updatePreference = (event: MediaQueryListEvent) => {
      setReducedMotion(event.matches);
      if (event.matches) onReduction();
    };
    setReducedMotion(query.matches);
    query.addEventListener("change", updatePreference);
    return () => query.removeEventListener("change", updatePreference);
  }, [onReduction]);
  return reducedMotion;
}

function useRegionVisibility() {
  const regionRef = useRef<HTMLElement>(null);
  const visibleRef = useRef(false);
  const [isVisible, setIsVisible] = useState(false);
  useEffect(() => {
    const region = regionRef.current;
    if (!region || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        const visible = entry?.isIntersecting ?? false;
        visibleRef.current = visible;
        setIsVisible(visible);
      },
      { threshold: 0.15 },
    );
    observer.observe(region);
    return () => observer.disconnect();
  }, []);
  return { isVisible, regionRef };
}

export function useCustomerDemoPlayback<State, World>({
  instance,
  nextDelayMs,
  state,
}: CustomerDemoPlaybackOptions<State, World>) {
  const autoplayAvailableRef = useRef(!prefersReducedMotion());
  const reducedMotionPlayRef = useRef(false);
  const wantsPlaybackRef = useRef(false);
  const { isVisible, regionRef } = useRegionVisibility();
  const handleReducedMotion = useCallback(() => {
    autoplayAvailableRef.current = false;
    reducedMotionPlayRef.current = false;
    wantsPlaybackRef.current = false;
    instance.commands.pause();
  }, [instance]);
  const reducedMotion = useReducedMotion(handleReducedMotion);

  useEffect(() => {
    if (state.sessionStatus.phase === "closed") {
      wantsPlaybackRef.current = false;
      if (state.playback.status === "playing") instance.commands.pause();
      return;
    }
    if (!isVisible || (reducedMotion && !reducedMotionPlayRef.current)) {
      if (state.playback.status === "playing") instance.commands.pause();
      return;
    }
    if (autoplayAvailableRef.current) {
      autoplayAvailableRef.current = false;
      wantsPlaybackRef.current = true;
    }
    if (
      wantsPlaybackRef.current &&
      state.playback.status === "paused" &&
      state.commandState === "idle" &&
      state.mode === "current"
    ) {
      instance.commands.play();
    }
  }, [instance, isVisible, reducedMotion, state]);

  useEffect(() => {
    if (
      state.playback.status !== "playing" ||
      state.commandState === "running" ||
      state.mode !== "current" ||
      state.sessionStatus.phase === "closed"
    ) {
      return;
    }
    const timer = window.setTimeout(
      () => {
        void instance.commands.step();
      },
      Math.max(0, nextDelayMs / state.playback.speed),
    );
    return () => window.clearTimeout(timer);
  }, [instance, nextDelayMs, state]);

  const pause = useCallback(() => {
    autoplayAvailableRef.current = false;
    reducedMotionPlayRef.current = false;
    wantsPlaybackRef.current = false;
    return instance.commands.pause();
  }, [instance]);

  const play = useCallback(() => {
    autoplayAvailableRef.current = false;
    reducedMotionPlayRef.current = true;
    wantsPlaybackRef.current = true;
    return instance.commands.play();
  }, [instance]);

  const selectTick = useCallback(
    (tick: number) => {
      autoplayAvailableRef.current = false;
      reducedMotionPlayRef.current = false;
      wantsPlaybackRef.current = false;
      return instance.commands.selectTick(tick);
    },
    [instance],
  );

  const step = useCallback(async () => {
    autoplayAvailableRef.current = false;
    reducedMotionPlayRef.current = false;
    wantsPlaybackRef.current = false;
    instance.commands.pause();
    return instance.commands.step();
  }, [instance]);

  const restart = useCallback(async () => {
    reducedMotionPlayRef.current = false;
    wantsPlaybackRef.current = false;
    autoplayAvailableRef.current = !prefersReducedMotion();
    const outcome = await instance.commands.restart();
    if (outcome.status === "accepted" && isVisible && !prefersReducedMotion()) {
      autoplayAvailableRef.current = false;
      wantsPlaybackRef.current = true;
      instance.commands.play();
    }
    return outcome;
  }, [instance, isVisible]);

  return { pause, play, regionRef, restart, selectTick, step };
}

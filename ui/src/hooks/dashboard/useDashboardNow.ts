import {
  useEffect,
  useState,
} from "react";

export function useDashboardNow(): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const interval = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => {
      window.clearInterval(interval);
    };
  }, []);

  return now;
}

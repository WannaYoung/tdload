import { onMounted, onUnmounted, ref } from "vue";

function readIsMobile(maxWidth: number): boolean {
  if (typeof window === "undefined") return false;
  return window.matchMedia(`(max-width: ${maxWidth}px)`).matches;
}

export function useMobile(maxWidth = 1000) {
  const isMobile = ref(readIsMobile(maxWidth));
  let mql: MediaQueryList | null = null;

  function sync() {
    isMobile.value = readIsMobile(maxWidth);
  }

  function onChange() {
    sync();
  }

  onMounted(() => {
    sync();
    mql = window.matchMedia(`(max-width: ${maxWidth}px)`);
    if (typeof mql.addEventListener === "function") {
      mql.addEventListener("change", onChange);
    } else {
      window.addEventListener("resize", onChange);
    }
  });

  onUnmounted(() => {
    if (mql && typeof mql.removeEventListener === "function") {
      mql.removeEventListener("change", onChange);
    } else if (typeof window !== "undefined") {
      window.removeEventListener("resize", onChange);
    }
    mql = null;
  });

  return isMobile;
}

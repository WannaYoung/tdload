import { ref } from "vue";

const PLACEHOLDER_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120" viewBox="0 0 120 120"><rect width="120" height="120" fill="#1f1f1f"/><rect x="1" y="1" width="118" height="118" fill="none" stroke="#3a3a3a"/><path d="M36 78l14-18 12 14 10-12 22 26H36z" fill="#444"/><circle cx="46" cy="46" r="8" fill="#444"/></svg>`;

export const imagePlaceholder = `data:image/svg+xml,${encodeURIComponent(PLACEHOLDER_SVG)}`;

const noImage = ref(false);

export function useNoImage() {
  return { noImage };
}

export function applyNoImageSetting(value: boolean) {
  noImage.value = value;
}

export function withImagePlaceholder(url: string): string {
  if (!url || noImage.value) return imagePlaceholder;
  return url;
}

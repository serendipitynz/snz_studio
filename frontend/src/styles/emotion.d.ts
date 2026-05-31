import "@emotion/react";
import type { ThemeTokens } from "./themes/types";

// Augment Emotion's Theme so `({ theme }) => theme.X` and useTheme() are typed
// against our ThemeTokens. Picked up automatically via tsconfig include: ["src"].
declare module "@emotion/react" {
  export interface Theme extends ThemeTokens {}
}

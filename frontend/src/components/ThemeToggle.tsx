import { Moon, Sun } from "lucide-react";
import { useTheme } from "../lib/theme-context";
import { IconButton } from "./ui";

export function ThemeToggle() {
  const { theme, toggleTheme } = useTheme();
  const nextLabel = theme === "dark" ? "Use light theme" : "Use dark theme";

  return (
    <IconButton label={nextLabel} onClick={toggleTheme}>
      {theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}
    </IconButton>
  );
}

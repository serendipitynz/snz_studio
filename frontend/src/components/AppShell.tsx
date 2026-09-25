import { useEffect } from "react";
import { Outlet } from "react-router-dom";
import { Container, Page } from "../styles/ui";

export function AppShell() {
  // A file dropped where no drop zone takes it would have the WebView open it in place
  // of the app (snz-design doc-9 §6.13). The zones prevent the default themselves, so
  // what reaches the window undecided is a drop outside every zone.
  useEffect(() => {
    const refuse = (event: DragEvent) => {
      if (event.defaultPrevented) {
        return;
      }
      event.preventDefault();
      if (event.dataTransfer) {
        event.dataTransfer.dropEffect = "none";
      }
    };
    window.addEventListener("dragover", refuse);
    window.addEventListener("drop", refuse);
    return () => {
      window.removeEventListener("dragover", refuse);
      window.removeEventListener("drop", refuse);
    };
  }, []);

  return (
    <Page>
      <Container>
        <Outlet />
      </Container>
    </Page>
  );
}

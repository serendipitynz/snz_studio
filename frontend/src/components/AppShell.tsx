import { Outlet } from "react-router-dom";
import { Container, Page } from "../styles/ui";

export function AppShell() {
  return (
    <Page>
      <Container>
        <Outlet />
      </Container>
    </Page>
  );
}

import * as React from "react";

const FloatingPortalContainerContext = React.createContext<
  React.RefObject<HTMLElement | null> | undefined
>(undefined);

export { FloatingPortalContainerContext };

import type { SVGProps } from "react";

type NavIconProps = SVGProps<SVGSVGElement>;

function NavIcon({ children, className, ...props }: NavIconProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
      {...props}
    >
      {children}
    </svg>
  );
}

/** Code graph: hub with satellites. */
export function GraphIcon({ className, ...props }: NavIconProps) {
  return (
    <NavIcon className={className} {...props}>
      <circle cx="12" cy="12" r="2.4" />
      <circle cx="12" cy="4.6" r="1.7" />
      <circle cx="5" cy="16.2" r="1.7" />
      <circle cx="19" cy="16.2" r="1.7" />
      <path d="M12 9.6V6.3" />
      <path d="M10.1 13.6 6.4 15.2" />
      <path d="M13.9 13.6 17.6 15.2" />
    </NavIcon>
  );
}

/** Sessions: a walk through the map. */
export function SessionsIcon({ className, ...props }: NavIconProps) {
  return (
    <NavIcon className={className} {...props}>
      <rect x="4" y="4" width="16" height="16" rx="3" />
      <circle cx="8.2" cy="9" r="1.15" />
      <circle cx="12" cy="14.4" r="1.15" />
      <circle cx="16.3" cy="9.8" r="1.15" />
      <path d="M9.2 9.8 11 13.1" />
      <path d="M13.1 13.6 15.3 10.8" />
    </NavIcon>
  );
}

/** Memory: stacked episodes, not a bookshelf. */
export function MemoryIcon({ className, ...props }: NavIconProps) {
  return (
    <NavIcon className={className} {...props}>
      <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" />
      <rect width="12" height="12" x="8" y="8" rx="2" />
      <path d="M11.2 13h5.6" />
      <path d="M11.2 16.2h3.6" />
    </NavIcon>
  );
}

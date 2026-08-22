"use client";

import { useLayoutEffect, useRef, type MutableRefObject } from "react";

/** Keep a ref equal to the latest value without assigning during render. */
export function useLatestRef<T>(value: T): MutableRefObject<T> {
  const ref = useRef(value);
  useLayoutEffect(() => {
    ref.current = value;
  });
  return ref;
}

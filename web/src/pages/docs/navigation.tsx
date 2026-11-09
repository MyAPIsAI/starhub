import { useEffect, useState, type RefObject } from "react";

export function ArticleOutline({
  article,
  language,
  articleRef,
  title,
}: {
  article: string;
  language: string;
  articleRef: RefObject<HTMLElement>;
  title: string;
}) {
  const [sections, setSections] = useState<{ id: string; text: string }[]>([]);
  const [active, setActive] = useState("");
  useEffect(() => {
    const main = articleRef.current;
    if (!main) return;
    const headings = Array.from(main.querySelectorAll<HTMLElement>("h2"));
    const elements = headings.length
      ? headings
      : Array.from(
          main.querySelectorAll<HTMLElement>(".docs-code > div > span"),
        );
    const rows = elements.map((element, index) => {
      element.id = `docs-${article}-section-${index}`;
      element.tabIndex = -1;
      element.classList.add("docs-anchor-target");
      return { id: element.id, text: element.textContent || "" };
    });
    setSections(rows);
    setActive(rows[0]?.id || "");
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries)
          if (entry.isIntersecting) setActive(entry.target.id);
      },
      { rootMargin: "-100px 0px -60% 0px" },
    );
    elements.forEach((element) => observer.observe(element));
    return () => observer.disconnect();
  }, [article, language, articleRef]);

  if (!sections.length) return null;
  return (
    <aside className="docs-outline">
      <nav aria-label={title}>
        <p>{title}</p>
        {sections.map((section) => (
          <a
            key={section.id}
            href={`#${section.id}`}
            aria-current={active === section.id ? "location" : undefined}
            onClick={() => {
              setActive(section.id);
              document
                .getElementById(section.id)
                ?.focus({ preventScroll: true });
            }}
          >
            {section.text}
          </a>
        ))}
      </nav>
    </aside>
  );
}

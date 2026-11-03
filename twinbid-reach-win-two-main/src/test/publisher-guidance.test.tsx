import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PublisherZones from "@/pages/PublisherZones";
import { PublisherPlacementNotice } from "@/components/publisher/PublisherPlacementNotice";
import { PublisherTestWrapper, publisherServer, codeFixture, userFixture, languageFixture } from "./publisher-fixtures";

vi.mock("@/contexts/AuthContext", () => ({useAuth:()=>({user:userFixture,signOut:vi.fn()})}));
vi.mock("@/contexts/LanguageContext", () => ({useLanguage:()=>languageFixture}));
beforeEach(()=> {localStorage.clear();languageFixture.lang="ru";});
afterEach(()=>{cleanup();vi.unstubAllGlobals();languageFixture.lang="ru";});

describe("Publisher installation and placement guidance",()=>{
  it.each([
  [
    "list",
    "ru",
    "Разные места — разные коды",
    "Если одинаковый код разместить в нескольких местах одного сайта — например, два баннера одного размера или несколько Popunder, — они склеятся в одну зону. Это правило действует для всех форматов."
  ],
  [
    "list",
    "en",
    "Different placements need different codes",
    "If the same code is used in multiple locations on one website—such as two banners of the same size or several Popunders—they merge into one zone. This applies to every format."
  ],
  [
    "list",
    "zh-CN",
    "不同展示位置需要不同代码",
    "同一代码用于一个网站的多个位置（例如两个相同尺寸的横幅或多个弹出广告）时，会合并为一个广告位。这适用于所有形式。"
  ],
  [
    "form",
    "ru",
    "Статистика мест размещения",
    "Один код можно использовать несколько раз на одном сайте: данные этих мест объединятся в одну зону. Для раздельной статистики и управления создайте или продублируйте код."
  ],
  [
    "form",
    "en",
    "Placement statistics",
    "One code can be reused on a website: its placements share one zone. Create or duplicate a code for separate statistics and controls."
  ],
  [
    "form",
    "zh-CN",
    "广告位统计",
    "同一代码可在网站上重复使用：这些位置共享一个广告位。若需分别统计和控制，请创建或复制代码。"
  ],
  [
    "installation",
    "ru",
    "Повторное использование кода",
    "Один CODE можно разместить несколько раз, но на одном домене статистика этих мест объединится. Для раздельного учёта создайте или продублируйте код."
  ],
  [
    "installation",
    "en",
    "Reusing a code",
    "You can place one CODE multiple times, but its placements on the same domain share statistics. Create or duplicate a code for separate reporting."
  ],
  [
    "installation",
    "zh-CN",
    "重复使用代码",
    "同一个 CODE 可多次放置，但同一域名上的统计会合并。若需分别统计，请创建或复制代码。"
  ]
])("restores the exact original %s copy in %s",(context,lang,title,text)=>{
    languageFixture.lang=lang;
    render(<PublisherPlacementNotice context={context as "list" | "form" | "installation"}/>);
    const notice=screen.getByRole("region",{name:title});
    expect(within(notice).getByText(text,{exact:true})).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
  it("shows the warning on the code list and in installation, alongside discovery and troubleshooting help",async()=>{
    publisherServer();
    render(<PublisherZones/>,{wrapper:PublisherTestWrapper});
    await screen.findByText(codeFixture.name);
    expect(screen.getByRole("region",{name:"Разные места — разные коды"})).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button",{name:"Код"}));
    const dialog=await screen.findByRole("dialog");
    expect(within(dialog).getByRole("region",{name:"Повторное использование кода"})).toHaveTextContent("Для раздельного учёта создайте или продублируйте код.");
    expect(within(dialog).getByRole("region",{name:"Что произойдёт после установки"})).toHaveTextContent("На одном домене каждому рекламному коду соответствует одна зона");
    expect(within(dialog).getByText(/Этот же код на другом домене/)).toBeInTheDocument();
    expect(within(dialog).getByText(/Добавьте тег один раз/)).toBeInTheDocument();
    const help=within(dialog).getByText("Если реклама не открывается").closest("details")!;
    expect(help.open).toBe(false);
    fireEvent.click(within(dialog).getByText("Если реклама не открывается"));
    expect(help.open).toBe(true);
    expect(help).toHaveTextContent("Блокировщик рекламы");
    expect(help).toHaveTextContent("iframe");
    expect(help).toHaveTextContent("каждые 5 минут");
    expect(dialog).toHaveTextContent(`food="${codeFixture.short_code}"`);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

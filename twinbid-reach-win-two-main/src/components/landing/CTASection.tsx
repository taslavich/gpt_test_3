import { ArrowRight } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { LineReveal } from "./CinematicReveal";

const ctaCopy = {
  ru: {
    section: "06 / ПЕРВОЕ ПОПОЛНЕНИЕ",
    eyebrow: "Больше возможностей на старте",
    lead: "Получите",
    tail: "к первому пополнению",
    text: "Один кабинет, один баланс и все инструменты для запуска и масштабирования кампаний.",
    register: "Создать аккаунт",
    contact: "Написать менеджеру",
    note: "Бонус начисляется на первое пополнение от $100",
    backdrop: "ПОПОЛНЕНИЕ → ЗАПУСК → МАСШТАБИРОВАНИЕ",
  },
  en: {
    section: "06 / FIRST DEPOSIT",
    eyebrow: "Give the first launch more power",
    lead: "We add",
    tail: "to your first deposit",
    text: "One account, one balance and a shorter route from an idea to a live campaign.",
    register: "Create an account",
    contact: "Talk to a manager",
    note: "The bonus applies to your first deposit from $100",
    backdrop: "DEPOSIT → TRAFFIC → SCALE",
  },
  es: {
    section: "06 / PRIMER DEPÓSITO",
    eyebrow: "Más potencia para el primer lanzamiento",
    lead: "Añadimos",
    tail: "a tu primer depósito",
    text: "Una cuenta, un saldo y una ruta más corta desde la idea hasta una campaña activa.",
    register: "Crear una cuenta",
    contact: "Hablar con un manager",
    note: "El bono se aplica al primer depósito desde $100",
    backdrop: "DEPÓSITO → TRÁFICO → ESCALA",
  },
  fr: {
    section: "06 / PREMIER DÉPÔT",
    eyebrow: "Plus de puissance pour le premier lancement",
    lead: "Nous ajoutons",
    tail: "à votre premier dépôt",
    text: "Un compte, un solde et un parcours plus court de l’idée à la campagne active.",
    register: "Créer un compte",
    contact: "Parler à un manager",
    note: "Le bonus s’applique au premier dépôt à partir de 100 $",
    backdrop: "DÉPÔT → TRAFIC → ÉCHELLE",
  },
};

export function CTASection() {
  const { lang } = useLanguage();
  const copy = ctaCopy[lang] ?? ctaCopy.en;

  return (
    <section className="editorial-cta">
      <div className="editorial-cta-backdrop" aria-hidden="true">
        <div className="editorial-cta-backdrop-track">
          <span>{copy.backdrop}</span>
          <span>{copy.backdrop}</span>
        </div>
      </div>
      <div className="editorial-cta-signal" aria-hidden="true"><i /><i /><i /><i /><i /></div>
      <div className="landing-editorial-shell editorial-cta-inner">
        <LineReveal>
          <div className="editorial-cta-kicker">
            <span>{copy.section}</span>
            <span>{copy.eyebrow}</span>
          </div>
        </LineReveal>

        <LineReveal delay={0.12}>
          <h2 className="editorial-cta-title">
            <span>{copy.lead}</span>
            <strong>+25%</strong>
            <span>{copy.tail}</span>
          </h2>
        </LineReveal>

        <div className="editorial-cta-bottom">
          <LineReveal delay={0.26} className="editorial-cta-copy">
            <p>{copy.text}</p>
            <small>{copy.note}</small>
          </LineReveal>
          <LineReveal delay={0.36} className="editorial-cta-actions">
              <AuthDialog
                defaultTab="register"
                trigger={<button className="landing-button landing-button-primary">{copy.register} <ArrowRight /></button>}
              />
              <a href="https://t.me/GregTwinbid" target="_blank" rel="noopener noreferrer" className="landing-button landing-button-ghost">{copy.contact} <ArrowRight /></a>
          </LineReveal>
        </div>
      </div>
    </section>
  );
}

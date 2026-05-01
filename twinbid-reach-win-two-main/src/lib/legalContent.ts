// Legal content (Terms of Use & Privacy Policy) localized in EN/RU.
// Source: provided official TwinBid documents.

type Block = {
  heading?: string;
  paragraphs?: string[];
  list?: string[];
};

type Doc = {
  title: string;
  sections: Block[];
};

type LegalContent = {
  terms: Doc;
  privacy: Doc;
};

const EN: LegalContent = {
  terms: {
    title: "Terms of Use",
    sections: [
      {
        paragraphs: [
          "These Terms of Use govern access to and use of the website https://twinbid.io/ and related TwinBid services.",
          "By accessing the website, creating an account, submitting information, or using TwinBid services, the user agrees to these Terms of Use.",
          "If you do not agree with these Terms of Use, you must not use the website or services.",
        ],
      },
      {
        heading: "1. General Information",
        paragraphs: [
          "TwinBid is an online advertising platform that provides tools for users to work with advertising campaigns, receive campaign-related notifications, and communicate with the TwinBid team.",
          "Website: https://twinbid.io/\nContact email: twinbid@twinbidex.com",
        ],
      },
      {
        heading: "2. Eligibility",
        paragraphs: ["By using TwinBid, the user confirms that:"],
        list: [
          "They are legally capable of accepting these Terms.",
          "They provide accurate and up-to-date information.",
          "They will use TwinBid only for lawful purposes.",
          "They will not use TwinBid in a way that violates applicable laws, third-party rights, or these Terms.",
        ],
      },
      {
        heading: "3. User Account",
        paragraphs: [
          "To access certain features, the user may need to create an account or provide contact information.",
          "The user is responsible for:",
        ],
        list: [
          "Providing accurate information.",
          "Maintaining the confidentiality of account credentials.",
          "All actions performed through their account.",
          "Promptly notifying TwinBid of any unauthorized access or security incident.",
        ],
      },
      {
        heading: "4. Advertising Campaigns",
        paragraphs: [
          "TwinBid may allow users to create, submit, manage, or monitor advertising campaigns.",
          "The user is solely responsible for:",
        ],
        list: [
          "The legality of advertising materials.",
          "The accuracy of campaign information.",
          "Compliance with applicable advertising laws and platform rules.",
          "Having all necessary rights, licenses, and permissions for submitted materials.",
          "Ensuring that advertising content does not violate third-party rights.",
        ],
      },
      {
        heading: "5. Prohibited Activities",
        paragraphs: ["The user must not use TwinBid for:"],
        list: [
          "Fraudulent or deceptive activity.",
          "Distribution of malware, phishing, scams, or harmful software.",
          "Promotion of illegal goods or services.",
          "Unauthorized collection of personal data.",
          "Violation of advertising, privacy, intellectual property, or consumer protection laws.",
          "Circumvention of moderation, security, anti-fraud, or technical restrictions.",
          "Use of bots, fake traffic, click fraud, impression fraud, or other artificial activity.",
          "Interference with the proper operation of the website or service.",
          "Attempting to gain unauthorized access to systems, accounts, or data.",
          "Uploading or distributing content that infringes rights of third parties.",
        ],
      },
      {
        heading: "6. Informational and Service Notifications",
        paragraphs: [
          "TwinBid may send users service notifications related to their account, advertising campaigns, moderation, campaign completion, technical updates, or other service-related matters.",
        ],
      },
      {
        heading: "7. Personal Data",
        paragraphs: [
          "TwinBid processes personal data in accordance with its Privacy Policy.",
          "By using the website or services and providing personal data, the user confirms that they have read the Privacy Policy.",
        ],
      },
      {
        heading: "8. Intellectual Property",
        paragraphs: [
          "All materials on the TwinBid website, including text, design, interface elements, logos, graphics, software elements, and other content, belong to TwinBid or its licensors unless otherwise stated.",
          "The user may not copy, reproduce, modify, distribute, sell, or use TwinBid materials without prior written permission, except where such use is expressly allowed by law or by these Terms.",
          "The user retains rights to the materials they submit to TwinBid, but grants TwinBid the right to use such materials to provide the service, process campaigns, perform moderation, display advertising, and operate the platform.",
        ],
      },
      {
        heading: "9. Availability of the Service",
        paragraphs: [
          "TwinBid aims to provide stable access to the website and services but does not guarantee uninterrupted, error-free, or always available operation.",
          "TwinBid may temporarily restrict or suspend access to the website or services for maintenance, updates, security reasons, technical issues, or other operational needs.",
        ],
      },
      {
        heading: "10. No Guaranteed Results",
        paragraphs: ["TwinBid does not guarantee specific advertising results, including but not limited to:"],
        list: [
          "Number of impressions.",
          "Number of clicks.",
          "Conversion rate.",
          "Revenue.",
          "Campaign profitability.",
          "User acquisition volume.",
          "Approval of any specific campaign.",
        ],
      },
      {
        heading: "11. Limitation of Liability",
        paragraphs: ["To the maximum extent permitted by applicable law, TwinBid is not liable for:"],
        list: [
          "Loss of profits, revenue, data, business opportunities, or goodwill.",
          "Indirect, incidental, special, or consequential damages.",
          "User's violation of laws or third-party rights.",
          "Advertising materials submitted by the user.",
          "Actions or omissions of third-party services, hosting providers, email providers, payment providers, advertisers, publishers, or traffic sources.",
          "Technical failures, interruptions, or security incidents outside TwinBid's reasonable control.",
        ],
      },
      {
        heading: "12. Suspension and Termination",
        paragraphs: ["TwinBid may suspend, restrict, or terminate the user's access to the website or services if:"],
        list: [
          "The user violates these Terms.",
          "The user provides false or misleading information.",
          "The user engages in suspicious, fraudulent, or harmful activity.",
          "TwinBid is required to do so by law or by competent authorities.",
          "Continued access may create legal, technical, financial, reputational, or security risks.",
        ],
      },
      {
        heading: "13. Third-Party Services",
        paragraphs: [
          "TwinBid may contain links to third-party websites, services, tools, or integrations.",
          "TwinBid is not responsible for the content, policies, actions, or availability of third-party services. The user interacts with third-party services at their own risk.",
        ],
      },
      {
        heading: "14. Contact Information",
        paragraphs: [
          "For questions related to these Terms of Use, please contact:\nEmail: twinbid@twinbidex.com\nWebsite: https://twinbid.io/",
        ],
      },
    ],
  },
  privacy: {
    title: "Privacy Policy",
    sections: [
      {
        paragraphs: [
          "This Privacy Policy explains how TwinBid processes personal data of users of the website https://twinbid.io/ and related services.",
          "By using the website, creating an account, submitting a form, or otherwise providing personal data to TwinBid, the user confirms that they have read this Privacy Policy.",
        ],
      },
      {
        heading: "1. General Information",
        paragraphs: [
          "TwinBid is an online advertising platform that allows users to interact with advertising campaign tools, receive service notifications, and communicate with the TwinBid team.",
          "Operator: TwinBid S.A.\nContact email: twinbid@twinbidex.com\nWebsite: https://twinbid.io/",
          "If you have any questions about this Privacy Policy or the processing of your personal data, you may contact us at: twinbid@twinbidex.com",
        ],
      },
      {
        heading: "2. Personal Data We Collect",
        paragraphs: ["TwinBid may collect and process the following personal data:"],
        list: [
          "Email address.",
          "Telegram username or Telegram contact.",
          "Username provided by the user.",
        ],
      },
      {
        heading: "3. Purposes of Processing Personal Data",
        paragraphs: ["TwinBid processes personal data for the following purposes:"],
        list: [
          "To create and maintain a user account.",
          "To identify the user within the TwinBid service.",
          "To communicate with the user regarding their account, requests, or use of the service.",
          "To send service notifications related to advertising campaigns, including notifications about campaign status, moderation, completion, or other campaign-related events.",
          "To send informational messages about TwinBid, its services, updates, features, and platform-related announcements, if the user has provided consent to receive such communications.",
          "To provide technical support.",
          "To ensure the security and proper operation of the website and service.",
        ],
      },
      {
        heading: "4. Legal Basis for Processing",
        paragraphs: ["TwinBid processes personal data on the basis of one or more of the following legal grounds:"],
        list: [
          "User consent.",
          "The need to provide access to the website and service.",
          "The need to communicate with the user regarding their use of TwinBid.",
          "Compliance with applicable legal obligations, where such obligations apply.",
        ],
      },
      {
        paragraphs: [
          "Where required by applicable law, TwinBid requests separate consent for the processing of personal data and separate consent for receiving informational or marketing communications.",
        ],
      },
      {
        heading: "5. Informational and Marketing Communications",
        paragraphs: ["TwinBid may use the user's email address to send:"],
        list: [
          "Service notifications related to the user's account and advertising campaigns.",
          "Notifications about campaign completion, moderation, status changes, or other service-related events.",
          "Informational messages about TwinBid updates, services, features, or offers.",
        ],
      },
      {
        heading: "6. Storage of Personal Data",
        paragraphs: [
          "TwinBid stores personal data only for as long as necessary for the purposes described in this Privacy Policy, unless a longer storage period is required or permitted by applicable law.",
          "Personal data may be deleted or anonymized when:",
        ],
        list: [
          "The user requests deletion of their data.",
          "The data is no longer necessary for the purposes for which it was collected.",
          "The user withdraws consent, where consent is the only legal basis for processing.",
          "TwinBid no longer needs the data for legal, security, or operational purposes.",
        ],
      },
      {
        heading: "7. Transfer of Personal Data to Third Parties",
        paragraphs: [
          "TwinBid does not sell users' personal data.",
          "TwinBid may transfer personal data to third-party service providers only when necessary for the operation of the website and service, including but not limited to:",
        ],
        list: [
          "Email delivery providers.",
          "Hosting providers.",
          "Analytics or technical infrastructure providers.",
          "Customer support tools.",
        ],
      },
      {
        paragraphs: [
          "Such third parties may process personal data only to the extent necessary to provide their services to TwinBid.",
          "TwinBid may also disclose personal data if required by applicable law, court order, or lawful request from competent authorities.",
        ],
      },
      {
        heading: "8. Data Security",
        paragraphs: [
          "TwinBid takes reasonable technical and organizational measures to protect personal data from unauthorized access, loss, misuse, alteration, or disclosure.",
          "However, no method of transmission or electronic storage is completely secure. Therefore, TwinBid cannot guarantee absolute security of personal data.",
        ],
      },
      {
        heading: "9. User Rights",
        paragraphs: ["Depending on applicable law, the user may have the right to:"],
        list: [
          "Request information about the processing of their personal data.",
          "Request access to their personal data.",
          "Request correction of inaccurate personal data.",
          "Request deletion of personal data.",
          "Withdraw consent to processing, where processing is based on consent.",
          "Object to receiving informational or marketing communications.",
          "Request restriction of processing where applicable.",
        ],
      },
      {
        heading: "10. Withdrawal of Consent",
        paragraphs: [
          "The user may withdraw consent to the processing of personal data at any time by contacting TwinBid at twinbid@twinbidex.com.",
          "Withdrawal of consent does not affect the lawfulness of processing carried out before the withdrawal.",
          "If the user withdraws consent necessary for the operation of the service, TwinBid may be unable to continue providing some or all services to that user.",
        ],
      },
      {
        heading: "11. Cookies and Technical Data",
        paragraphs: [
          "TwinBid may use cookies and similar technologies to ensure the proper functioning of the website, improve user experience, analyze website usage, and maintain security.",
        ],
      },
      {
        heading: "12. Changes to this Privacy Policy",
        paragraphs: [
          "TwinBid may update this Privacy Policy from time to time.",
        ],
      },
      {
        heading: "13. Contact Information",
        paragraphs: [
          "For questions related to this Privacy Policy or personal data processing, please contact:\nEmail: twinbid@twinbidex.com\nWebsite: https://twinbid.io/",
        ],
      },
    ],
  },
};

const RU: LegalContent = {
  terms: {
    title: "Условия использования",
    sections: [
      {
        paragraphs: [
          "Настоящие Условия использования регулируют доступ и использование сайта https://twinbid.io/ и связанных сервисов TwinBid.",
          "Используя сайт, создавая аккаунт, отправляя информацию или используя сервисы TwinBid, пользователь соглашается с настоящими Условиями.",
          "Если вы не согласны с настоящими Условиями, вы не должны использовать сайт или сервисы.",
        ],
      },
      {
        heading: "1. Общая информация",
        paragraphs: [
          "TwinBid — онлайн-платформа интернет-рекламы, предоставляющая инструменты для работы с рекламными кампаниями, получения уведомлений и связи с командой TwinBid.",
          "Сайт: https://twinbid.io/\nКонтактный email: twinbid@twinbidex.com",
        ],
      },
      {
        heading: "2. Право использования",
        paragraphs: ["Используя TwinBid, пользователь подтверждает, что:"],
        list: [
          "Юридически способен принять настоящие Условия.",
          "Предоставляет точную и актуальную информацию.",
          "Будет использовать TwinBid только в законных целях.",
          "Не будет использовать TwinBid способом, нарушающим применимое законодательство, права третьих лиц или настоящие Условия.",
        ],
      },
      {
        heading: "3. Учётная запись пользователя",
        paragraphs: [
          "Для доступа к ряду функций пользователю может потребоваться создать аккаунт или предоставить контактные данные.",
          "Пользователь несёт ответственность за:",
        ],
        list: [
          "Предоставление достоверной информации.",
          "Сохранение конфиденциальности учётных данных.",
          "Все действия, совершённые через его аккаунт.",
          "Своевременное уведомление TwinBid о любом несанкционированном доступе или инциденте безопасности.",
        ],
      },
      {
        heading: "4. Рекламные кампании",
        paragraphs: [
          "TwinBid может позволять пользователям создавать, отправлять, управлять и контролировать рекламные кампании.",
          "Пользователь несёт исключительную ответственность за:",
        ],
        list: [
          "Законность рекламных материалов.",
          "Точность информации о кампании.",
          "Соблюдение применимых законов о рекламе и правил платформы.",
          "Наличие всех необходимых прав, лицензий и разрешений на отправленные материалы.",
          "Гарантию того, что рекламный контент не нарушает права третьих лиц.",
        ],
      },
      {
        heading: "5. Запрещённые действия",
        paragraphs: ["Пользователь не должен использовать TwinBid для:"],
        list: [
          "Мошеннической или вводящей в заблуждение деятельности.",
          "Распространения вредоносного ПО, фишинга, скамов или вредных программ.",
          "Продвижения нелегальных товаров или услуг.",
          "Несанкционированного сбора персональных данных.",
          "Нарушения законов о рекламе, конфиденциальности, интеллектуальной собственности или защите потребителей.",
          "Обхода модерации, безопасности, антифрода или технических ограничений.",
          "Использования ботов, фейкового трафика, кликфрода или другой искусственной активности.",
          "Помех нормальной работе сайта или сервиса.",
          "Попыток несанкционированного доступа к системам, аккаунтам или данным.",
          "Загрузки или распространения контента, нарушающего права третьих лиц.",
        ],
      },
      {
        heading: "6. Информационные и сервисные уведомления",
        paragraphs: [
          "TwinBid может отправлять пользователям сервисные уведомления, связанные с их аккаунтом, рекламными кампаниями, модерацией, завершением кампаний, техническими обновлениями и другими сервисными вопросами.",
        ],
      },
      {
        heading: "7. Персональные данные",
        paragraphs: [
          "TwinBid обрабатывает персональные данные в соответствии со своей Политикой конфиденциальности.",
          "Используя сайт или сервисы и предоставляя персональные данные, пользователь подтверждает, что ознакомился с Политикой конфиденциальности.",
        ],
      },
      {
        heading: "8. Интеллектуальная собственность",
        paragraphs: [
          "Все материалы на сайте TwinBid, включая текст, дизайн, элементы интерфейса, логотипы, графику, программные элементы и другой контент, принадлежат TwinBid или его лицензиарам, если не указано иное.",
          "Пользователь не вправе копировать, воспроизводить, изменять, распространять, продавать или использовать материалы TwinBid без предварительного письменного разрешения.",
          "Пользователь сохраняет права на материалы, которые он отправляет в TwinBid, но предоставляет TwinBid право использовать их для оказания сервиса, обработки кампаний, модерации и работы платформы.",
        ],
      },
      {
        heading: "9. Доступность сервиса",
        paragraphs: [
          "TwinBid стремится обеспечить стабильный доступ к сайту и сервисам, но не гарантирует бесперебойную, безошибочную или постоянно доступную работу.",
          "TwinBid может временно ограничивать или приостанавливать доступ к сайту или сервисам для обслуживания, обновлений, безопасности, технических причин или других операционных нужд.",
        ],
      },
      {
        heading: "10. Отсутствие гарантированных результатов",
        paragraphs: ["TwinBid не гарантирует конкретных рекламных результатов, в том числе:"],
        list: [
          "Количество показов.",
          "Количество кликов.",
          "Коэффициент конверсии.",
          "Доход.",
          "Прибыльность кампании.",
          "Объём привлечённых пользователей.",
          "Одобрение какой-либо конкретной кампании.",
        ],
      },
      {
        heading: "11. Ограничение ответственности",
        paragraphs: ["В максимально допустимой законом степени TwinBid не несёт ответственности за:"],
        list: [
          "Потерю прибыли, выручки, данных, бизнес-возможностей или деловой репутации.",
          "Косвенные, случайные, специальные или последующие убытки.",
          "Нарушение пользователем законов или прав третьих лиц.",
          "Рекламные материалы, отправленные пользователем.",
          "Действия или бездействие сторонних сервисов, хостинг-провайдеров, email-провайдеров, платёжных провайдеров, рекламодателей, паблишеров или источников трафика.",
          "Технические сбои, перерывы или инциденты безопасности вне разумного контроля TwinBid.",
        ],
      },
      {
        heading: "12. Приостановка и прекращение",
        paragraphs: ["TwinBid может приостановить, ограничить или прекратить доступ пользователя к сайту или сервисам, если:"],
        list: [
          "Пользователь нарушает настоящие Условия.",
          "Пользователь предоставляет ложную или вводящую в заблуждение информацию.",
          "Пользователь занимается подозрительной, мошеннической или вредной деятельностью.",
          "TwinBid обязан сделать это по закону или по запросу компетентных органов.",
          "Продолжение доступа может создать юридические, технические, финансовые, репутационные или связанные с безопасностью риски.",
        ],
      },
      {
        heading: "13. Сторонние сервисы",
        paragraphs: [
          "TwinBid может содержать ссылки на сторонние сайты, сервисы, инструменты или интеграции.",
          "TwinBid не несёт ответственности за содержание, политики, действия или доступность сторонних сервисов. Пользователь взаимодействует со сторонними сервисами на свой риск.",
        ],
      },
      {
        heading: "14. Контактная информация",
        paragraphs: [
          "По вопросам, связанным с настоящими Условиями использования, обращайтесь:\nEmail: twinbid@twinbidex.com\nСайт: https://twinbid.io/",
        ],
      },
    ],
  },
  privacy: {
    title: "Политика конфиденциальности",
    sections: [
      {
        paragraphs: [
          "Настоящая Политика конфиденциальности описывает, как TwinBid обрабатывает персональные данные пользователей сайта https://twinbid.io/ и связанных сервисов.",
          "Используя сайт, создавая аккаунт, отправляя форму или иным образом предоставляя персональные данные TwinBid, пользователь подтверждает, что ознакомился с настоящей Политикой.",
        ],
      },
      {
        heading: "1. Общая информация",
        paragraphs: [
          "TwinBid — онлайн-платформа интернет-рекламы, позволяющая пользователям работать с инструментами рекламных кампаний, получать сервисные уведомления и общаться с командой TwinBid.",
          "Оператор: TwinBid S.A.\nКонтактный email: twinbid@twinbidex.com\nСайт: https://twinbid.io/",
          "По любым вопросам, связанным с настоящей Политикой или обработкой персональных данных, можно связаться с нами по адресу: twinbid@twinbidex.com",
        ],
      },
      {
        heading: "2. Персональные данные, которые мы собираем",
        paragraphs: ["TwinBid может собирать и обрабатывать следующие персональные данные:"],
        list: [
          "Email-адрес.",
          "Имя пользователя в Telegram или контакт в Telegram.",
          "Имя пользователя, указанное им самим.",
        ],
      },
      {
        heading: "3. Цели обработки персональных данных",
        paragraphs: ["TwinBid обрабатывает персональные данные в следующих целях:"],
        list: [
          "Для создания и поддержки аккаунта пользователя.",
          "Для идентификации пользователя в сервисе TwinBid.",
          "Для коммуникации с пользователем по поводу его аккаунта, обращений или использования сервиса.",
          "Для отправки сервисных уведомлений, связанных с рекламными кампаниями, включая уведомления о статусе кампании, модерации, завершении или иных событиях, связанных с кампанией.",
          "Для отправки информационных сообщений о TwinBid, его сервисах, обновлениях, функциях и анонсах платформы при наличии согласия пользователя на получение таких сообщений.",
          "Для оказания технической поддержки.",
          "Для обеспечения безопасности и корректной работы сайта и сервиса.",
        ],
      },
      {
        heading: "4. Правовые основания обработки",
        paragraphs: ["TwinBid обрабатывает персональные данные на одном или нескольких следующих правовых основаниях:"],
        list: [
          "Согласие пользователя.",
          "Необходимость предоставления доступа к сайту и сервису.",
          "Необходимость коммуникации с пользователем по поводу использования TwinBid.",
          "Соблюдение применимых юридических обязательств, если такие обязательства применимы.",
        ],
      },
      {
        paragraphs: [
          "В случаях, когда это требуется применимым законодательством, TwinBid запрашивает отдельное согласие на обработку персональных данных и отдельное согласие на получение информационных или маркетинговых сообщений.",
        ],
      },
      {
        heading: "5. Информационные и маркетинговые сообщения",
        paragraphs: ["TwinBid может использовать email-адрес пользователя для отправки:"],
        list: [
          "Сервисных уведомлений, связанных с аккаунтом и рекламными кампаниями.",
          "Уведомлений о завершении кампаний, модерации, изменениях статуса или других сервисных событиях.",
          "Информационных сообщений об обновлениях, услугах, функциях или предложениях TwinBid.",
        ],
      },
      {
        heading: "6. Хранение персональных данных",
        paragraphs: [
          "TwinBid хранит персональные данные только в течение времени, необходимого для целей, описанных в настоящей Политике, если более длительный срок хранения не требуется или не разрешён применимым законом.",
          "Персональные данные могут быть удалены или анонимизированы, когда:",
        ],
        list: [
          "Пользователь запрашивает удаление своих данных.",
          "Данные больше не необходимы для целей, для которых они были собраны.",
          "Пользователь отзывает согласие, если согласие является единственным правовым основанием обработки.",
          "TwinBid больше не нуждается в данных для юридических, операционных целей или целей безопасности.",
        ],
      },
      {
        heading: "7. Передача персональных данных третьим лицам",
        paragraphs: [
          "TwinBid не продаёт персональные данные пользователей.",
          "TwinBid может передавать персональные данные сторонним поставщикам услуг только в случае необходимости для работы сайта и сервиса, включая, но не ограничиваясь:",
        ],
        list: [
          "Поставщики email-доставки.",
          "Хостинг-провайдеры.",
          "Поставщики аналитики или технической инфраструктуры.",
          "Инструменты поддержки клиентов.",
        ],
      },
      {
        paragraphs: [
          "Такие третьи лица могут обрабатывать персональные данные только в объёме, необходимом для оказания их услуг TwinBid.",
          "TwinBid также может раскрывать персональные данные, если это требуется применимым законодательством, решением суда или законным запросом компетентных органов.",
        ],
      },
      {
        heading: "8. Безопасность данных",
        paragraphs: [
          "TwinBid принимает разумные технические и организационные меры для защиты персональных данных от несанкционированного доступа, потери, неправомерного использования, изменения или раскрытия.",
          "Однако ни один способ передачи или электронного хранения не является полностью безопасным. Поэтому TwinBid не может гарантировать абсолютную безопасность персональных данных.",
        ],
      },
      {
        heading: "9. Права пользователя",
        paragraphs: ["В зависимости от применимого законодательства пользователь может иметь право:"],
        list: [
          "Запросить информацию об обработке своих персональных данных.",
          "Запросить доступ к своим персональным данным.",
          "Запросить исправление неточных персональных данных.",
          "Запросить удаление персональных данных.",
          "Отозвать согласие на обработку, если обработка основана на согласии.",
          "Возразить против получения информационных или маркетинговых сообщений.",
          "Запросить ограничение обработки, где это применимо.",
        ],
      },
      {
        heading: "10. Отзыв согласия",
        paragraphs: [
          "Пользователь может отозвать согласие на обработку персональных данных в любое время, обратившись в TwinBid по адресу twinbid@twinbidex.com.",
          "Отзыв согласия не влияет на законность обработки, осуществлённой до отзыва.",
          "Если пользователь отзывает согласие, необходимое для работы сервиса, TwinBid может быть не в состоянии продолжать предоставлять некоторые или все услуги этому пользователю.",
        ],
      },
      {
        heading: "11. Файлы cookie и технические данные",
        paragraphs: [
          "TwinBid может использовать файлы cookie и аналогичные технологии для обеспечения корректной работы сайта, улучшения пользовательского опыта, анализа использования сайта и поддержания безопасности.",
        ],
      },
      {
        heading: "12. Изменения в настоящей Политике",
        paragraphs: [
          "TwinBid может периодически обновлять настоящую Политику конфиденциальности.",
        ],
      },
      {
        heading: "13. Контактная информация",
        paragraphs: [
          "По вопросам, связанным с настоящей Политикой конфиденциальности или обработкой персональных данных, обращайтесь:\nEmail: twinbid@twinbidex.com\nСайт: https://twinbid.io/",
        ],
      },
    ],
  },
};

export const LEGAL_CONTENT = { en: EN, ru: RU } as const;
